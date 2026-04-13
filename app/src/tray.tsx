import React, { useState, useEffect, useRef } from "react";
import ReactDOM from "react-dom/client";
import { readDir, exists } from "@tauri-apps/plugin-fs";
import { homeDir } from "@tauri-apps/api/path";
import { spawn } from "tauri-pty";

interface Profile {
  name: string;
}

function TrayPanel() {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [selectedProfile, setSelectedProfile] = useState("lecture");
  const [label, setLabel] = useState("");
  const [extraPrompt, setExtraPrompt] = useState("");
  const [isRecording, setIsRecording] = useState(false);
  const [statusLines, setStatusLines] = useState<string[]>([]);
  const [elapsed, setElapsed] = useState(0);
  const ptyRef = useRef<any>(null);
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    loadProfiles();
  }, []);

  async function loadProfiles() {
    try {
      const home = await homeDir();
      const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
      const profileDir = `${homePath}/.notate/profiles`;
      const dirExists = await exists(profileDir);
      if (!dirExists) return;

      const entries = await readDir(profileDir);
      const list: Profile[] = entries
        .filter((e) => e.name.endsWith(".md"))
        .map((e) => ({ name: e.name.replace(".md", "") }))
        .sort((a, b) => a.name.localeCompare(b.name));

      setProfiles(list);
    } catch (err) {
      console.error("Failed to load profiles:", err);
    }
  }

  async function startCapture(mode: "L" | "LT" | "LTW") {
    const args: string[] = [mode];
    if (label.trim()) {
      args.push("-l", label.trim());
    }
    if (mode === "LTW") {
      args.push("-p", selectedProfile);
      if (extraPrompt.trim()) {
        args.push("-e", extraPrompt.trim());
      }
    }

    try {
      const home = await homeDir();
      const homePath = home.endsWith("/") ? home.slice(0, -1) : home;

      const pty = spawn("/usr/local/bin/autonote", args, {
        cwd: homePath,
        cols: 80,
        rows: 24,
      });

      // Capture output for status display
      const decoder = new TextDecoder();
      pty.onData((data: Uint8Array) => {
        const text = decoder.decode(data);
        const clean = text.replace(/\x1b\[[0-9;]*[a-zA-Z]/g, "");
        const lines = clean.split(/[\r\n]+/)
          .map((l) => l.trim())
          .filter((l) => {
            if (!l) return false;
            if (l.startsWith("✓") || l.startsWith("⟳") || l.startsWith("⏹")) return true;
            if (l.startsWith("Profile:")) return true;
            if (l.includes("Done") || l.includes("Error") || l.includes("error")) return true;
            return false;
          });
        if (lines.length > 0) {
          setStatusLines((prev) => [...prev, ...lines].slice(-6));
        }
      });

      pty.onExit(() => {
        cleanup();
        setStatusLines((prev) => [...prev, "Session ended."].slice(-6));
      });

      ptyRef.current = pty;
      setIsRecording(true);
      setElapsed(0);
      setStatusLines([]);

      timerRef.current = setInterval(() => {
        setElapsed((prev) => prev + 1);
      }, 1000);
    } catch (err) {
      console.error("Failed to start capture:", err);
      setStatusLines([`Error: ${String(err)}`]);
    }
  }

  function stopCapture() {
    if (!ptyRef.current) return;

    // Send Ctrl+C for graceful shutdown
    ptyRef.current.write("\x03");
    setStatusLines((prev) => [...prev, "⏹ Stopping..."].slice(-6));

    // Force kill after 30 seconds if still running
    setTimeout(() => {
      if (ptyRef.current) {
        ptyRef.current.kill();
        cleanup();
      }
    }, 30000);
  }

  function cleanup() {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
    ptyRef.current = null;
    setIsRecording(false);
  }

  return (
    <div className="tray-panel">
      <div className="tray-header">
        <span className="tray-title">AutoNote</span>
        {isRecording && (
          <span className="tray-recording">
            {Math.floor(elapsed / 60)}:{(elapsed % 60).toString().padStart(2, "0")}
          </span>
        )}
      </div>

      {isRecording ? (
        <div className="tray-recording-state">
          <div className="tray-output">
            {statusLines.map((line, i) => (
              <p key={i} className="tray-output-line">{line}</p>
            ))}
          </div>
          <button className="tray-btn stop" onClick={stopCapture}>
            Stop
          </button>
        </div>
      ) : (
        <>
          <div className="tray-section">
            <label className="tray-label">Session Name</label>
            <input
              type="text"
              className="tray-input"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="e.g., lecture, meeting"
            />
          </div>

          <div className="tray-section">
            <label className="tray-label">Profile</label>
            <select
              className="tray-select"
              value={selectedProfile}
              onChange={(e) => setSelectedProfile(e.target.value)}
            >
              {profiles.map((p) => (
                <option key={p.name} value={p.name}>{p.name}</option>
              ))}
            </select>
          </div>

          <div className="tray-section">
            <label className="tray-label">Extra Prompt (optional)</label>
            <textarea
              className="tray-textarea"
              value={extraPrompt}
              onChange={(e) => setExtraPrompt(e.target.value)}
              placeholder="Additional instructions..."
              rows={2}
            />
          </div>

          <div className="tray-actions">
            <button className="tray-btn" onClick={() => startCapture("L")}>
              Capture
            </button>
            <button className="tray-btn" onClick={() => startCapture("LT")}>
              Transcribe
            </button>
            <button className="tray-btn primary" onClick={() => startCapture("LTW")}>
              Write
            </button>
          </div>
        </>
      )}
    </div>
  );
}

const style = document.createElement("style");
style.textContent = `
  * { margin: 0; padding: 0; box-sizing: border-box; }

  html, body {
    background: transparent !important;
    background-color: rgba(0, 0, 0, 0) !important;
    height: 100%;
    overflow: hidden;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
    font-size: 13px;
    -webkit-user-select: none;
    user-select: none;
  }

  #tray-root { background: transparent; }

  .tray-panel {
    background: #1a1a1a;
    border: none;
    border-radius: 10px;
    padding: 14px;
    margin: 0;
    color: #aaa;
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .tray-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 12px;
    border-bottom: 1px solid #2a2a2a;
    margin-bottom: 12px;
    flex-shrink: 0;
  }

  .tray-title { font-weight: 700; font-size: 14px; color: #e0e0e0; }

  .tray-recording {
    font-size: 12px;
    color: #f44;
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 6px;
    font-family: 'SF Mono', Menlo, monospace;
  }
  .tray-recording::before {
    content: '';
    width: 6px;
    height: 6px;
    background: #f44;
    border-radius: 50%;
    animation: pulse 1.5s infinite;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.3; }
  }

  .tray-section { margin-bottom: 12px; }

  .tray-label {
    display: block;
    font-size: 11px;
    color: #666;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 4px;
  }

  .tray-input, .tray-select, .tray-textarea {
    width: 100%;
    background: #111;
    border: 1px solid #2a2a2a;
    color: #ccc;
    padding: 7px 10px;
    border-radius: 6px;
    font-size: 13px;
    font-family: inherit;
    outline: none;
  }

  .tray-input:focus, .tray-select:focus, .tray-textarea:focus { border-color: #4fc1ff; }
  .tray-input::placeholder, .tray-textarea::placeholder { color: #444; }
  .tray-textarea { resize: none; line-height: 1.4; }

  .tray-select {
    cursor: pointer;
    -webkit-appearance: none;
    appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg width='10' height='6' viewBox='0 0 10 6' fill='none' xmlns='http://www.w3.org/2000/svg'%3E%3Cpath d='M1 1L5 5L9 1' stroke='%23666' stroke-width='1.5' stroke-linecap='round'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right 10px center;
    padding-right: 28px;
  }

  .tray-actions { display: flex; gap: 6px; }

  .tray-btn {
    flex: 1;
    padding: 8px 0;
    background: #222;
    border: 1px solid #2a2a2a;
    border-radius: 6px;
    color: #bbb;
    cursor: pointer;
    text-align: center;
    font-size: 12px;
    font-weight: 600;
  }
  .tray-btn:hover { background: #2a2a2a; border-color: #444; color: #e0e0e0; }

  .tray-btn.primary {
    background: rgba(79, 193, 255, 0.1);
    border-color: rgba(79, 193, 255, 0.25);
    color: #4fc1ff;
  }
  .tray-btn.primary:hover {
    background: rgba(79, 193, 255, 0.18);
    border-color: rgba(79, 193, 255, 0.4);
  }

  .tray-btn.stop {
    background: rgba(244, 68, 68, 0.1);
    border-color: rgba(244, 68, 68, 0.3);
    color: #f44;
    text-align: center;
    padding: 10px;
    font-weight: 600;
    font-size: 13px;
    flex-shrink: 0;
  }
  .tray-btn.stop:hover { background: rgba(244, 68, 68, 0.2); }

  .tray-recording-state { display: flex; flex-direction: column; gap: 12px; flex: 1; }
  .tray-output { flex: 1; overflow-y: auto; }

  .tray-output-line {
    font-size: 11px;
    color: #888;
    font-family: 'SF Mono', Menlo, monospace;
    line-height: 1.5;
    word-break: break-all;
  }
`;
document.head.appendChild(style);

document.addEventListener("contextmenu", (e) => e.preventDefault());

ReactDOM.createRoot(document.getElementById("tray-root") as HTMLElement).render(
  <React.StrictMode>
    <TrayPanel />
  </React.StrictMode>
);
