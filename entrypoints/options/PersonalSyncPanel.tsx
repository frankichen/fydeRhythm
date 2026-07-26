import React, { useEffect, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Divider from "@mui/material/Divider";
import FormControl from "@mui/material/FormControl";
import FormControlLabel from "@mui/material/FormControlLabel";
import InputLabel from "@mui/material/InputLabel";
import MenuItem from "@mui/material/MenuItem";
import Select from "@mui/material/Select";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";
import Typography from "@mui/material/Typography";

import styles from "./styles.module.less";
import {
    PERSONAL_SYNC_STATUS_KEY,
    deleteLocalLexicon,
    kDefaultPersonalSyncSettings,
    kDefaultPersonalSyncStatus,
    listLocalLexicon,
    loadPersonalSyncSettings,
    loadPersonalSyncStatus,
    resetPersonalSyncLocalData,
    savePersonalSyncSettings,
    syncPersonalDataNow,
    testPersonalSyncConnection,
    upsertLocalLexicon,
    type LexiconEntry,
    type PersonalSyncSettings,
    type PersonalSyncStatus,
} from "@/lib/personal-sync";

function formatTime(value: number): string {
    if (!value) return "尚未同步";
    return new Date(value).toLocaleString("zh-CN", { hour12: false });
}

function statusLabel(status: PersonalSyncStatus): string {
    if (status.state === "syncing") return "正在同步";
    if (status.state === "error") return "同步异常";
    if (status.state === "disabled") return "未启用";
    return "已就绪";
}

export default function PersonalSyncPanel() {
    const [settings, setSettings] = useState<PersonalSyncSettings>(kDefaultPersonalSyncSettings);
    const [status, setStatus] = useState<PersonalSyncStatus>(kDefaultPersonalSyncStatus);
    const [lexicon, setLexicon] = useState<LexiconEntry[]>([]);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);
    const [message, setMessage] = useState("");
    const [error, setError] = useState("");
    const [phrase, setPhrase] = useState("");
    const [code, setCode] = useState("");
    const [shortcut, setShortcut] = useState("");
    const [category, setCategory] = useState("");
    const [weight, setWeight] = useState(100);

    const refreshLexicon = async () => setLexicon((await listLocalLexicon()).slice(0, 100));

    useEffect(() => {
        let alive = true;
        void Promise.all([
            loadPersonalSyncSettings(),
            loadPersonalSyncStatus(),
            listLocalLexicon(),
        ]).then(([nextSettings, nextStatus, entries]) => {
            if (!alive) return;
            setSettings(nextSettings);
            setStatus(nextStatus);
            setLexicon(entries.slice(0, 100));
        }).catch((reason) => {
            if (alive) setError(String(reason));
        }).finally(() => {
            if (alive) setLoading(false);
        });

        const onStorageChanged = (changes: Record<string, chrome.storage.StorageChange>, area: string) => {
            if (area !== "local" || !changes[PERSONAL_SYNC_STATUS_KEY]) return;
            void loadPersonalSyncStatus().then((next) => {
                if (alive) setStatus(next);
            });
        };
        chrome.storage.onChanged.addListener(onStorageChanged);
        return () => {
            alive = false;
            chrome.storage.onChanged.removeListener(onStorageChanged);
        };
    }, []);

    const modeHelp = useMemo(() => {
        if (settings.mode === "download-only") return "只从服务器下载，不把本机输入习惯上传。适合公司电脑。";
        if (settings.mode === "manual") return "只在点击“立即同步”时上传和下载。";
        return "本地即时学习，空闲时批量上传，并每 5 分钟拉取其他设备变更。";
    }, [settings.mode]);

    const save = async () => {
        setBusy(true);
        setMessage("");
        setError("");
        try {
            const next = await savePersonalSyncSettings(settings);
            setSettings(next);
            setMessage("同步设置已保存。API Token 仅保存在本机扩展存储中。");
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
        } finally {
            setBusy(false);
        }
    };

    const testConnection = async () => {
        setBusy(true);
        setMessage("");
        setError("");
        try {
            const next = await savePersonalSyncSettings(settings);
            setSettings(next);
            const result = await testPersonalSyncConnection(next);
            setMessage(`服务器连接成功，本地待上传 ${result.pending} 条，当前游标 ${result.cursor}。`);
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
        } finally {
            setBusy(false);
        }
    };

    const syncNow = async () => {
        setBusy(true);
        setMessage("");
        setError("");
        try {
            const next = await savePersonalSyncSettings(settings);
            setSettings(next);
            const nextStatus = await syncPersonalDataNow();
            setStatus(nextStatus);
            await refreshLexicon();
            setMessage(`同步完成，服务器游标 ${nextStatus.cursor}，待上传 ${nextStatus.pendingCount} 条。`);
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
            setStatus(await loadPersonalSyncStatus());
        } finally {
            setBusy(false);
        }
    };

    const addLexicon = async () => {
        setBusy(true);
        setMessage("");
        setError("");
        try {
            await upsertLocalLexicon({ phrase, code, shortcut, category, weight });
            setPhrase("");
            setCode("");
            setShortcut("");
            setCategory("");
            setWeight(100);
            await refreshLexicon();
            setMessage("个人词已保存到本地；完整同步模式下会在后台上传。");
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
        } finally {
            setBusy(false);
        }
    };

    const removeLexicon = async (id: string) => {
        setBusy(true);
        setMessage("");
        setError("");
        try {
            await deleteLocalLexicon(id);
            await refreshLexicon();
            setMessage("个人词已删除，并生成跨设备删除记录。");
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
        } finally {
            setBusy(false);
        }
    };

    const resetLocal = async () => {
        if (!confirm("确定清空本机同步缓存吗？服务器数据不会被删除，下次同步会重新下载。")) return;
        setBusy(true);
        setMessage("");
        setError("");
        try {
            await resetPersonalSyncLocalData();
            setStatus(kDefaultPersonalSyncStatus);
            setLexicon([]);
            setMessage("本机缓存已清空。点击“立即同步”可重新下载服务器数据。");
        } catch (reason) {
            setError(reason instanceof Error ? reason.message : String(reason));
        } finally {
            setBusy(false);
        }
    };

    if (loading) {
        return <div className={styles.formGroup}><div className={styles.formBox}><CircularProgress size={24} /></div></div>;
    }

    return <div className={styles.formGroup}>
        <div className={styles.formBox}>
            <FormControl className={styles.formControl} fullWidth>
                <Stack spacing={2}>
                    <Box>
                        <Typography variant="h6">个人词库与输入习惯同步</Typography>
                        <Typography variant="body2" color="text.secondary">
                            输入时只读本地数据，网络仅用于后台增量同步；服务器不可用不会影响正常打字。
                        </Typography>
                    </Box>

                    <FormControlLabel
                        control={<Switch checked={settings.enabled} onChange={(_, checked) => setSettings({ ...settings, enabled: checked })} />}
                        label="启用个人同步"
                    />

                    <TextField
                        label="同步服务器"
                        value={settings.serverUrl}
                        onChange={(event) => setSettings({ ...settings, serverUrl: event.target.value })}
                        helperText="默认使用已部署的 https://shulufa.555044.xyz"
                        fullWidth
                    />
                    <TextField
                        label="API Token"
                        type="password"
                        value={settings.apiToken}
                        onChange={(event) => setSettings({ ...settings, apiToken: event.target.value })}
                        helperText="Token 只保存在 chrome.storage.local，不进入 Chrome 同步。"
                        autoComplete="off"
                        fullWidth
                    />
                    <Stack direction={{ xs: "column", md: "row" }} spacing={2}>
                        <TextField
                            label="设备名称"
                            value={settings.deviceName}
                            onChange={(event) => setSettings({ ...settings, deviceName: event.target.value })}
                            fullWidth
                        />
                        <TextField
                            label="设备 ID"
                            value={settings.deviceId}
                            onChange={(event) => setSettings({ ...settings, deviceId: event.target.value })}
                            helperText="自动生成，除非设备重置，否则不要修改。"
                            fullWidth
                        />
                    </Stack>

                    <FormControl fullWidth>
                        <InputLabel id="personal-sync-mode-label">同步模式</InputLabel>
                        <Select
                            labelId="personal-sync-mode-label"
                            label="同步模式"
                            value={settings.mode}
                            onChange={(event) => setSettings({ ...settings, mode: event.target.value as PersonalSyncSettings["mode"] })}
                        >
                            <MenuItem value="full">完整同步</MenuItem>
                            <MenuItem value="download-only">仅下载</MenuItem>
                            <MenuItem value="manual">仅手动同步</MenuItem>
                        </Select>
                        <Typography variant="caption" color="text.secondary" sx={{ mt: 0.5 }}>{modeHelp}</Typography>
                    </FormControl>

                    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        <Button variant="contained" onClick={save} disabled={busy}>保存设置</Button>
                        <Button variant="outlined" onClick={testConnection} disabled={busy || !settings.apiToken}>测试连接</Button>
                        <Button variant="outlined" onClick={syncNow} disabled={busy || !settings.enabled || !settings.apiToken}>立即同步</Button>
                        <Button color="warning" onClick={resetLocal} disabled={busy}>清空本机缓存</Button>
                        {busy && <CircularProgress size={24} />}
                    </Stack>

                    <Box sx={{ p: 1.5, border: "1px solid", borderColor: "divider", borderRadius: 1 }}>
                        <Typography variant="body2">状态：{statusLabel(status)}</Typography>
                        <Typography variant="body2">最近同步：{formatTime(status.lastSyncAt)}</Typography>
                        <Typography variant="body2">待上传：{status.pendingCount} 条　服务器游标：{status.cursor}</Typography>
                    </Box>

                    {message && <Alert severity="success" onClose={() => setMessage("")}>{message}</Alert>}
                    {(error || status.error) && <Alert severity="error" onClose={() => setError("")}>{error || status.error}</Alert>}

                    <Divider />

                    <Box>
                        <Typography variant="h6">个人词库</Typography>
                        <Typography variant="body2" color="text.secondary">
                            输入编码或缩写完全匹配时，候选窗会显示个人词，按 Tab 接受；普通 Rime 候选顺序不会被打乱。
                        </Typography>
                    </Box>

                    <Stack direction={{ xs: "column", md: "row" }} spacing={1}>
                        <TextField label="词语" value={phrase} onChange={(event) => setPhrase(event.target.value)} fullWidth />
                        <TextField label="五笔/输入编码" value={code} onChange={(event) => setCode(event.target.value)} fullWidth />
                        <TextField label="快捷缩写" value={shortcut} onChange={(event) => setShortcut(event.target.value)} fullWidth />
                    </Stack>
                    <Stack direction={{ xs: "column", md: "row" }} spacing={1}>
                        <TextField label="分类" value={category} onChange={(event) => setCategory(event.target.value)} fullWidth />
                        <TextField
                            label="权重"
                            type="number"
                            value={weight}
                            onChange={(event) => setWeight(Number(event.target.value) || 0)}
                            fullWidth
                        />
                        <Button variant="contained" onClick={addLexicon} disabled={busy || !phrase.trim()} sx={{ minWidth: 120 }}>添加</Button>
                    </Stack>

                    <Stack spacing={1}>
                        {lexicon.length === 0 && <Typography variant="body2" color="text.secondary">还没有个人词。</Typography>}
                        {lexicon.map((entry) => <Box
                            key={entry.id}
                            sx={{ display: "flex", alignItems: "center", gap: 1, p: 1, border: "1px solid", borderColor: "divider", borderRadius: 1 }}
                        >
                            <Box sx={{ flex: 1, minWidth: 0 }}>
                                <Typography variant="body1" noWrap>{entry.phrase}</Typography>
                                <Typography variant="caption" color="text.secondary" noWrap>
                                    编码：{entry.code || "-"}　缩写：{entry.shortcut || "-"}　权重：{entry.weight}{entry.category ? `　${entry.category}` : ""}
                                </Typography>
                            </Box>
                            <Button color="error" size="small" onClick={() => removeLexicon(entry.id)} disabled={busy}>删除</Button>
                        </Box>)}
                    </Stack>
                </Stack>
            </FormControl>
        </div>
    </div>;
}
