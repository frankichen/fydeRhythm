#include <algorithm>
#include <cstdlib>
#include <atomic>
#include <cerrno>
#include <condition_variable>
#include <cstring>
#include <deque>
#include <memory>
#include <mutex>
#include <sstream>
#include <string>
#include <thread>
#include <unordered_map>
#include <utility>
#include <vector>

#include <sys/socket.h>
#include <sys/time.h>
#include <sys/un.h>
#include <unistd.h>

#include <fcitx-utils/capabilityflags.h>
#include <fcitx-utils/eventdispatcher.h>
#include <fcitx-utils/key.h>
#include <fcitx-utils/trackableobject.h>
#include <fcitx-utils/utf8.h>
#include <fcitx/addonfactory.h>
#include <fcitx/addonmanager.h>
#include <fcitx/addoninstance.h>
#include <fcitx/candidatelist.h>
#include <fcitx/event.h>
#include <fcitx/inputcontext.h>
#include <fcitx/inputpanel.h>
#include <fcitx/instance.h>
#include <fcitx/text.h>
#include <fcitx/userinterface.h>

using namespace fcitx;

namespace {

constexpr size_t kMaxSocketBody = 1 << 20;
constexpr size_t kCorrectionContextChars = 600;
constexpr size_t kPredictionContextChars = 1200;

struct SourceSnapshot {
    std::string allText;
    unsigned int cursor = 0;
    unsigned int anchor = 0;
    int deleteOffset = 0;
    unsigned int deleteSize = 0;
    std::string source;
};

struct AIResult {
    std::string action;
    SourceSnapshot snapshot;
    std::string output;
};

struct Task {
    TrackableObjectReference<InputContext> inputContext;
    std::string action;
    SourceSnapshot snapshot;
};

std::string socketPath() {
    const char *runtime = std::getenv("XDG_RUNTIME_DIR");
    if (runtime && *runtime) {
        return std::string(runtime) + "/fyderhythm-ai.sock";
    }
    return "/tmp/fyderhythm-ai-" + std::to_string(getuid()) + ".sock";
}

bool writeAll(int fd, const void *buffer, size_t length) {
    const char *data = static_cast<const char *>(buffer);
    while (length > 0) {
        const auto written = ::write(fd, data, length);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            return false;
        }
        data += written;
        length -= static_cast<size_t>(written);
    }
    return true;
}

bool readAll(int fd, void *buffer, size_t length) {
    char *data = static_cast<char *>(buffer);
    while (length > 0) {
        const auto count = ::read(fd, data, length);
        if (count == 0) {
            return false;
        }
        if (count < 0) {
            if (errno == EINTR) {
                continue;
            }
            return false;
        }
        data += count;
        length -= static_cast<size_t>(count);
    }
    return true;
}

bool readLine(int fd, std::string &line) {
    line.clear();
    while (line.size() < 4096) {
        char ch = 0;
        const auto count = ::read(fd, &ch, 1);
        if (count == 0) {
            return false;
        }
        if (count < 0) {
            if (errno == EINTR) {
                continue;
            }
            return false;
        }
        if (ch == '\n') {
            return true;
        }
        line.push_back(ch);
    }
    return false;
}

std::pair<bool, std::string> callDaemon(const std::string &action,
                                        const std::string &text) {
    const int fd = ::socket(AF_UNIX, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (fd < 0) {
        return {false, "无法创建 AI 本地连接"};
    }
    struct timeval timeout {};
    timeout.tv_sec = 70;
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));
    setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout));

    sockaddr_un addr {};
    addr.sun_family = AF_UNIX;
    const auto path = socketPath();
    if (path.size() >= sizeof(addr.sun_path)) {
        ::close(fd);
        return {false, "AI socket 路径过长"};
    }
    std::strncpy(addr.sun_path, path.c_str(), sizeof(addr.sun_path) - 1);
    if (::connect(fd, reinterpret_cast<sockaddr *>(&addr), sizeof(addr)) < 0) {
        const auto message = std::string("AI 服务未运行：") + std::strerror(errno);
        ::close(fd);
        return {false, message};
    }

    std::ostringstream request;
    request << "FYDEAI/1 " << action << " " << text.size() << "\n";
    const auto header = request.str();
    if (!writeAll(fd, header.data(), header.size()) ||
        !writeAll(fd, text.data(), text.size())) {
        ::close(fd);
        return {false, "发送 AI 请求失败"};
    }

    std::string responseHeader;
    if (!readLine(fd, responseHeader)) {
        ::close(fd);
        return {false, "读取 AI 响应头失败"};
    }
    std::istringstream parser(responseHeader);
    std::string status;
    size_t length = 0;
    parser >> status >> length;
    if (!parser || length > kMaxSocketBody) {
        ::close(fd);
        return {false, "AI 响应协议不正确"};
    }
    std::string body(length, '\0');
    if (length > 0 && !readAll(fd, body.data(), length)) {
        ::close(fd);
        return {false, "读取 AI 响应正文失败"};
    }
    ::close(fd);
    return {status == "OK", body};
}

std::vector<size_t> charOffsets(const std::string &text) {
    std::vector<size_t> offsets;
    offsets.push_back(0);
    auto iter = text.begin();
    while (iter != text.end()) {
        uint32_t chr = 0;
        iter = utf8::getNextChar(iter, text.end(), &chr);
        if (!utf8::isValidChar(chr)) {
            return {};
        }
        offsets.push_back(static_cast<size_t>(std::distance(text.begin(), iter)));
    }
    return offsets;
}

bool isBoundary(uint32_t chr) {
    return chr == 0x3002 || chr == 0xFF01 || chr == 0xFF1F || chr == '!' ||
           chr == '?' || chr == '\n' || chr == '\r';
}

bool sourceForCorrection(InputContext *ic, SourceSnapshot &snapshot) {
    const auto &surrounding = ic->surroundingText();
    if (!surrounding.isValid()) {
        return false;
    }
    snapshot.allText = surrounding.text();
    snapshot.cursor = surrounding.cursor();
    snapshot.anchor = surrounding.anchor();
    const auto offsets = charOffsets(snapshot.allText);
    if (offsets.empty() || snapshot.cursor >= offsets.size() ||
        snapshot.anchor >= offsets.size()) {
        return false;
    }

    const auto startSelection = std::min(snapshot.cursor, snapshot.anchor);
    const auto endSelection = std::max(snapshot.cursor, snapshot.anchor);
    if (startSelection != endSelection) {
        snapshot.source = snapshot.allText.substr(offsets[startSelection],
                                                  offsets[endSelection] - offsets[startSelection]);
        snapshot.deleteOffset = static_cast<int>(startSelection) -
                                static_cast<int>(snapshot.cursor);
        snapshot.deleteSize = endSelection - startSelection;
        return !snapshot.source.empty();
    }

    unsigned int start = snapshot.cursor;
    unsigned int scanned = 0;
    while (start > 0 && scanned < kCorrectionContextChars) {
        const auto byteStart = offsets[start - 1];
        const auto byteEnd = offsets[start];
        const auto chr = utf8::getChar(snapshot.allText.begin() + byteStart,
                                       snapshot.allText.begin() + byteEnd);
        if (scanned > 0 && isBoundary(chr)) {
            break;
        }
        --start;
        ++scanned;
    }
    while (start < snapshot.cursor) {
        const auto byteStart = offsets[start];
        const auto byteEnd = offsets[start + 1];
        const auto chr = utf8::getChar(snapshot.allText.begin() + byteStart,
                                       snapshot.allText.begin() + byteEnd);
        if (chr != ' ' && chr != '\t' && chr != '\n' && chr != '\r') {
            break;
        }
        ++start;
    }
    if (start == snapshot.cursor) {
        return false;
    }
    snapshot.source = snapshot.allText.substr(offsets[start],
                                              offsets[snapshot.cursor] - offsets[start]);
    snapshot.deleteOffset = static_cast<int>(start) -
                            static_cast<int>(snapshot.cursor);
    snapshot.deleteSize = snapshot.cursor - start;
    return !snapshot.source.empty();
}

bool sourceForPrediction(InputContext *ic, SourceSnapshot &snapshot) {
    const auto &surrounding = ic->surroundingText();
    if (!surrounding.isValid()) {
        return false;
    }
    snapshot.allText = surrounding.text();
    snapshot.cursor = surrounding.cursor();
    snapshot.anchor = surrounding.anchor();
    const auto offsets = charOffsets(snapshot.allText);
    if (offsets.empty() || snapshot.cursor >= offsets.size() ||
        snapshot.anchor >= offsets.size()) {
        return false;
    }
    unsigned int start = 0;
    if (snapshot.cursor > kPredictionContextChars) {
        start = snapshot.cursor - kPredictionContextChars;
    }
    snapshot.source = snapshot.allText.substr(offsets[start],
                                              offsets[snapshot.cursor] - offsets[start]);
    snapshot.deleteOffset = 0;
    snapshot.deleteSize = 0;
    return !snapshot.source.empty();
}

bool unchanged(InputContext *ic, const SourceSnapshot &snapshot) {
    const auto &surrounding = ic->surroundingText();
    return surrounding.isValid() && surrounding.text() == snapshot.allText &&
           surrounding.cursor() == snapshot.cursor &&
           surrounding.anchor() == snapshot.anchor;
}

} // namespace

class FydeRhythmAI;

class AIWord final : public CandidateWord {
public:
    AIWord(FydeRhythmAI *owner, const std::string &output);
    void select(InputContext *inputContext) const override;

private:
    FydeRhythmAI *owner_;
};

class FydeRhythmAI final : public AddonInstance {
public:
    explicit FydeRhythmAI(Instance *instance) : instance_(instance) {
        dispatcher_.attach(&instance_->eventLoop());
        eventHandlers_.emplace_back(instance_->watchEvent(
            EventType::InputContextKeyEvent, EventWatcherPhase::PreInputMethod,
            [this](Event &event) { handleKey(static_cast<KeyEvent &>(event)); }));
        eventHandlers_.emplace_back(instance_->watchEvent(
            EventType::InputContextFocusOut, EventWatcherPhase::Default,
            [this](Event &event) {
                clear(static_cast<InputContextEvent &>(event).inputContext());
            }));
        eventHandlers_.emplace_back(instance_->watchEvent(
            EventType::InputContextDestroyed, EventWatcherPhase::Default,
            [this](Event &event) {
                clear(static_cast<InputContextEvent &>(event).inputContext());
            }));
        worker_ = std::thread([this]() { workerLoop(); });
    }

    ~FydeRhythmAI() override {
        alive_->store(false);
        {
            std::lock_guard<std::mutex> lock(queueMutex_);
            stopping_ = true;
        }
        queueCondition_.notify_all();
        if (worker_.joinable()) {
            worker_.join();
        }
        dispatcher_.detach();
    }

    void apply(InputContext *ic) {
        auto iter = results_.find(ic);
        if (iter == results_.end()) {
            return;
        }
        const auto result = iter->second;
        if (!unchanged(ic, result.snapshot)) {
            showMessage(ic, "上下文已变化，请重新触发 AI");
            results_.erase(iter);
            return;
        }
        if (result.action == "correct") {
            ic->deleteSurroundingText(result.snapshot.deleteOffset,
                                      result.snapshot.deleteSize);
        }
        ic->commitString(result.output);
        clear(ic);
    }

private:
    void handleKey(KeyEvent &event) {
        if (event.isRelease()) {
            return;
        }
        auto *ic = event.inputContext();
        auto resultIter = results_.find(ic);
        if (resultIter != results_.end()) {
            if (event.key().check(FcitxKey_Escape)) {
                clear(ic);
                event.filterAndAccept();
                return;
            }
            if (event.key().check(FcitxKey_1) ||
                event.key().check(FcitxKey_Return) ||
                event.key().check(FcitxKey_KP_Enter) ||
                event.key().check(FcitxKey_space)) {
                apply(ic);
                event.filterAndAccept();
                return;
            }
        }

        std::string action;
        if (event.key().check(Key("Alt+R")) ||
            event.key().check(Key("Control+Alt+R"))) {
            action = "correct";
        } else if (event.key().check(Key("Alt+Return")) ||
                   event.key().check(Key("Alt+KP_Enter")) ||
                   event.key().check(Key("Control+Alt+Return")) ||
                   event.key().check(Key("Control+Alt+KP_Enter"))) {
            action = "predict";
        } else {
            return;
        }
        event.filterAndAccept();

        if (ic->capabilityFlags().testAny(CapabilityFlag::PasswordOrSensitive)) {
            showMessage(ic, "敏感输入框中已禁用 AI");
            return;
        }
        const auto candidateList = ic->inputPanel().candidateList();
        if (!ic->inputPanel().preedit().empty() ||
            (candidateList && !candidateList->empty())) {
            showMessage(ic, "请先完成当前输入，再触发 AI");
            return;
        }
        if (!ic->capabilityFlags().test(CapabilityFlag::SurroundingText)) {
            showMessage(ic, "当前应用不支持读取光标附近文本");
            return;
        }

        SourceSnapshot snapshot;
        const bool ok = action == "correct" ? sourceForCorrection(ic, snapshot)
                                              : sourceForPrediction(ic, snapshot);
        if (!ok) {
            showMessage(ic, action == "correct" ? "请选择文字，或把光标放在句末"
                                                  : "光标前没有可用于续写的上下文");
            return;
        }
        showMessage(ic, action == "correct" ? "DeepSeek 正在纠错…"
                                              : "DeepSeek 正在续写…");
        {
            std::lock_guard<std::mutex> lock(queueMutex_);
            queue_.push_back(Task{ic->watch(), action, std::move(snapshot)});
            if (queue_.size() > 4) {
                queue_.pop_front();
            }
        }
        queueCondition_.notify_one();
    }

    void workerLoop() {
        while (true) {
            Task task;
            {
                std::unique_lock<std::mutex> lock(queueMutex_);
                queueCondition_.wait(lock, [this]() {
                    return stopping_ || !queue_.empty();
                });
                if (stopping_ && queue_.empty()) {
                    return;
                }
                task = std::move(queue_.front());
                queue_.pop_front();
            }
            auto response = callDaemon(task.action, task.snapshot.source);
            auto alive = alive_;
            dispatcher_.schedule([this, alive, task = std::move(task),
                                  response = std::move(response)]() mutable {
                if (!alive->load()) {
                    return;
                }
                auto *ic = task.inputContext.get();
                if (!ic) {
                    return;
                }
                if (!response.first) {
                    showMessage(ic, response.second);
                    return;
                }
                if (!unchanged(ic, task.snapshot)) {
                    showMessage(ic, "上下文已变化，已丢弃过期 AI 结果");
                    return;
                }
                AIResult result{task.action, std::move(task.snapshot),
                                std::move(response.second)};
                results_[ic] = std::move(result);
                showCandidate(ic);
            });
        }
    }

    void showCandidate(InputContext *ic) {
        auto iter = results_.find(ic);
        if (iter == results_.end()) {
            return;
        }
        auto candidates = std::make_unique<CommonCandidateList>();
        candidates->setPageSize(1);
        candidates->setLabels({"1"});
        candidates->append<AIWord>(this, iter->second.output);
        candidates->setGlobalCursorIndex(0);
        ic->inputPanel().setAuxUp(Text(iter->second.action == "correct"
                                           ? "DeepSeek 纠错候选（回车/空格/1 接受，Esc 取消）"
                                           : "DeepSeek 续写候选（回车/空格/1 接受，Esc 取消）"));
        ic->inputPanel().setCandidateList(std::move(candidates));
        ic->updateUserInterface(UserInterfaceComponent::InputPanel, true);
    }

    void showMessage(InputContext *ic, const std::string &message) {
        ic->inputPanel().setAuxUp(Text(message));
        ic->inputPanel().setCandidateList(nullptr);
        ic->updateUserInterface(UserInterfaceComponent::InputPanel, true);
    }

    void clear(InputContext *ic) {
        results_.erase(ic);
        if (ic) {
            ic->inputPanel().setAuxUp(Text());
            ic->inputPanel().setCandidateList(nullptr);
            ic->updateUserInterface(UserInterfaceComponent::InputPanel, true);
        }
    }

    Instance *instance_;
    EventDispatcher dispatcher_;
    std::vector<std::unique_ptr<HandlerTableEntry<EventHandler>>> eventHandlers_;
    std::unordered_map<InputContext *, AIResult> results_;
    std::mutex queueMutex_;
    std::condition_variable queueCondition_;
    std::deque<Task> queue_;
    bool stopping_ = false;
    std::thread worker_;
    std::shared_ptr<std::atomic_bool> alive_ =
        std::make_shared<std::atomic_bool>(true);

    friend class AIWord;
};

AIWord::AIWord(FydeRhythmAI *owner, const std::string &output)
    : CandidateWord(Text(output)), owner_(owner) {
}

void AIWord::select(InputContext *inputContext) const { owner_->apply(inputContext); }

class FydeRhythmAIFactory final : public AddonFactory {
public:
    AddonInstance *create(AddonManager *manager) override {
        return new FydeRhythmAI(manager->instance());
    }
};

extern "C" __attribute__((visibility("default"))) AddonFactory *
fcitx_addon_factory_instance() {
    static FydeRhythmAIFactory factory;
    return &factory;
}
