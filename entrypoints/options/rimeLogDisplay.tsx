import { sendMessage } from "@/lib/messaging";
import { useEffect, useRef, useState } from "react";

function RimeLogDisplay() {
    const [rimeLogs, setRimeLogs] = useState<string[]>([]);
    const logTextArea = useRef<HTMLTextAreaElement | null>(null);

    useEffect(() => {
        // Scroll textarea to bottom on log update
        const area = logTextArea.current;
        if (!area) return;
        area.scrollTop = area.scrollHeight;
    }, [rimeLogs]);

    async function updateRimeLogs() {
        const result = await sendMessage("GetRimeLogs");
        setRimeLogs(result.logs);
    }
    
    useEffect(() => {
        updateRimeLogs();

        const listener = (m: { rimeLog?: string }) => {
            if (m.rimeLog) {
                setRimeLogs(rimeLogs => [...rimeLogs, m.rimeLog!]);
            }
        };
        chrome.runtime.onMessage.addListener(listener);

        return () => {
            chrome.runtime.onMessage.removeListener(listener);
        };
    }, []);

    return <textarea readOnly value={rimeLogs.join("\n")} rows={14} ref={logTextArea}></textarea>;
}

export default RimeLogDisplay
