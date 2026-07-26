import { useEffect, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import FormControlLabel from "@mui/material/FormControlLabel";
import MenuItem from "@mui/material/MenuItem";
import Stack from "@mui/material/Stack";
import Switch from "@mui/material/Switch";
import TextField from "@mui/material/TextField";

import {
    kDefaultDeepSeekSettings,
    loadDeepSeekSettings,
    normalizeDeepSeekSettings,
    saveDeepSeekSettings,
    testDeepSeekSettings,
    type DeepSeekSettings,
} from "@/lib/deepseek";

export default function DeepSeekSettingsPanel() {
    const [settings, setSettings] = useState<DeepSeekSettings>(kDefaultDeepSeekSettings);
    const [status, setStatus] = useState("");
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        void loadDeepSeekSettings().then(setSettings).catch((error) => {
            setStatus(`读取设置失败：${String(error)}`);
        });
    }, []);

    function change(patch: Partial<DeepSeekSettings>) {
        setSettings((current) => normalizeDeepSeekSettings({ ...current, ...patch }));
        setStatus("设置尚未保存");
    }

    async function save() {
        setBusy(true);
        try {
            await saveDeepSeekSettings(settings);
            setStatus(settings.enabled ? "已保存，AI 候选排序已开启" : "已保存，AI 候选排序处于关闭状态");
        } catch (error) {
            setStatus(`保存失败：${String(error)}`);
        } finally {
            setBusy(false);
        }
    }

    async function test() {
        setBusy(true);
        setStatus("正在测试 DeepSeek 连接……");
        try {
            const order = await testDeepSeekSettings(settings);
            setStatus(`连接成功，测试排序结果：${order.join(" → ")}`);
        } catch (error) {
            setStatus(`连接失败：${error instanceof Error ? error.message : String(error)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <Box>
            <Box sx={{ fontSize: "1.05rem", fontWeight: 600, mb: 1 }}>DeepSeek 智能候选排序（试验）</Box>
            <Box sx={{ color: "text.secondary", fontSize: "0.9rem", mb: 2, lineHeight: 1.7 }}>
                Rime 候选会立即显示；停止输入约 0.2 秒后，DeepSeek 根据光标前文重新排序。
                请求超时、断网或接口异常时自动保留本地候选。密码、网址、邮箱、电话、数字和禁止学习的输入框不会调用 AI。
            </Box>

            <Stack spacing={2}>
                <FormControlLabel
                    control={
                        <Switch
                            checked={settings.enabled}
                            onChange={(_, checked) => change({ enabled: checked })}
                        />
                    }
                    label="启用 DeepSeek 候选排序"
                />

                <TextField
                    label="DeepSeek API Key"
                    type="password"
                    value={settings.apiKey}
                    onChange={(event) => change({ apiKey: event.target.value })}
                    helperText="密钥只保存在这台 Chromebook 的扩展本地存储中，不会提交到 GitHub。"
                    fullWidth
                />

                <TextField
                    select
                    label="模型"
                    value={settings.model}
                    onChange={(event) => change({ model: event.target.value as DeepSeekSettings["model"] })}
                    fullWidth
                >
                    <MenuItem value="deepseek-v4-flash">DeepSeek V4 Flash（推荐，速度快）</MenuItem>
                    <MenuItem value="deepseek-v4-pro">DeepSeek V4 Pro（更贵、通常更慢）</MenuItem>
                </TextField>

                <Stack direction={{ xs: "column", sm: "row" }} spacing={2}>
                    <TextField
                        label="超时（毫秒）"
                        type="number"
                        value={settings.timeoutMs}
                        onChange={(event) => change({ timeoutMs: Number(event.target.value) })}
                        inputProps={{ min: 250, max: 3000, step: 50 }}
                        fullWidth
                    />
                    <TextField
                        label="停止输入后等待（毫秒）"
                        type="number"
                        value={settings.debounceMs}
                        onChange={(event) => change({ debounceMs: Number(event.target.value) })}
                        inputProps={{ min: 100, max: 1000, step: 20 }}
                        fullWidth
                    />
                    <TextField
                        label="发送前文字符数"
                        type="number"
                        value={settings.maxContextChars}
                        onChange={(event) => change({ maxContextChars: Number(event.target.value) })}
                        inputProps={{ min: 0, max: 100, step: 5 }}
                        fullWidth
                    />
                </Stack>

                <Stack direction="row" spacing={1.5}>
                    <Button variant="contained" onClick={() => void save()} disabled={busy}>
                        保存 AI 设置
                    </Button>
                    <Button variant="outlined" onClick={() => void test()} disabled={busy || !settings.apiKey.trim()}>
                        测试连接
                    </Button>
                </Stack>

                {status && (
                    <Box sx={{ color: status.includes("失败") ? "error.main" : "text.secondary", fontSize: "0.9rem" }}>
                        {status}
                    </Box>
                )}
            </Stack>
        </Box>
    );
}
