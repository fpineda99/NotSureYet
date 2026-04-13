import { useEffect, useState, useRef } from "react";
import { readTextFile, writeTextFile, exists } from "@tauri-apps/plugin-fs";
import { openPath } from "@tauri-apps/plugin-opener";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { spawn } from "tauri-pty";
import "@xterm/xterm/css/xterm.css";
import type { Session, Profile } from "../App";

interface NotateState {
  session_id: string;
  last_segment_index: number;
  last_processed_at: string;
  profile: string;
}

interface Props {
  session: Session | null;
  profiles: Profile[];
  onViewTranscript: (session: Session) => void;
  onViewJSON: (session: Session) => void;
  onViewNotes: (session: Session) => void;
}

export default function Terminal({ session, profiles, onViewTranscript, onViewJSON, onViewNotes }: Props) {
  const [activeTab, setActiveTab] = useState<"chat" | "info">("chat");
  const [state, setState] = useState<NotateState | null>(null);

  useEffect(() => {
    if (!session) {
      setState(null);
      return;
    }
    loadState();
  }, [session]);

  async function loadState() {
    if (!session) return;
    try {
      const statePath = `${session.path}/.notate-state.json`;
      const fileExists = await exists(statePath);
      if (fileExists) {
        const content = await readTextFile(statePath);
        setState(JSON.parse(content));
      } else {
        setState(null);
      }
    } catch (err) {
      console.error("Failed to load session state:", err);
      setState(null);
    }
  }

  async function handleProfileChange(newProfile: string) {
    if (!session) return;

    // Only update the profile field — preserve all other state
    const statePath = `${session.path}/.notate-state.json`;
    const updated: NotateState = state
      ? { ...state, profile: newProfile, last_processed_at: new Date().toISOString() }
      : { session_id: "", last_segment_index: -1, last_processed_at: new Date().toISOString(), profile: newProfile };

    try {
      await writeTextFile(statePath, JSON.stringify(updated, null, 2));
      setState(updated);
    } catch (err) {
      console.error("Failed to update profile:", err);
    }
  }

  return (
    <div className="terminal-panel">
      <div className="tab-bar">
        <button
          className={`tab ${activeTab === "chat" ? "active" : ""}`}
          onClick={() => setActiveTab("chat")}
        >
          Chat
        </button>
        <button
          className={`tab ${activeTab === "info" ? "active" : ""}`}
          onClick={() => setActiveTab("info")}
        >
          Info
        </button>
      </div>

      {activeTab === "chat" ? (
        <ChatTab session={session} state={state} />
      ) : (
        <InfoTab
          session={session}
          state={state}
          profiles={profiles}
          onProfileChange={handleProfileChange}
          onViewTranscript={onViewTranscript}
          onViewJSON={onViewJSON}
          onViewNotes={onViewNotes}
        />
      )}
    </div>
  );
}

// Info tab — clean session details + file actions
function InfoTab({
  session,
  state,
  profiles,
  onProfileChange,
  onViewTranscript,
  onViewJSON,
  onViewNotes,
}: {
  session: Session | null;
  state: NotateState | null;
  profiles: Profile[];
  onProfileChange: (profile: string) => void;
  onViewTranscript: (session: Session) => void;
  onViewJSON: (session: Session) => void;
  onViewNotes: (session: Session) => void;
}) {
  if (!session) {
    return (
      <div className="terminal-content">
        <p className="empty">Select a session to view details.</p>
      </div>
    );
  }

  return (
    <div className="terminal-content">
      <div className="session-info">
        <div className="info-row">
          <span className="info-label">Session</span>
          <span className="info-value">{session.name}</span>
        </div>

        {state?.last_processed_at && (
          <div className="info-row">
            <span className="info-label">Last Updated</span>
            <span className="info-value">
              {new Date(state.last_processed_at).toLocaleString()}
            </span>
          </div>
        )}

        <div className="info-row">
          <span className="info-label">Profile</span>
          <select
            className="profile-select"
            value={state?.profile ?? "lecture"}
            onChange={(e) => onProfileChange(e.target.value)}
          >
            {profiles.map((p) => (
              <option key={p.name} value={p.name}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
      </div>

      <div className="file-actions">
        <span className="info-label">Files</span>

        <div className="file-grid">
          <button
            className="file-grid-btn"
            disabled={!session.hasRecording}
            onClick={async () => {
              try {
                await openPath(session.path + "/recording.wav");
              } catch (err) {
                console.error("Failed to play audio:", err);
              }
            }}
          >
            .wav
          </button>
          <button
            className="file-grid-btn"
            disabled={!session.hasTranscript}
            onClick={() => onViewTranscript(session)}
          >
            Transcript
          </button>
          <button
            className="file-grid-btn"
            disabled={!session.hasTranscript}
            onClick={() => onViewJSON(session)}
          >
            JSON
          </button>
          <button
            className="file-grid-btn"
            disabled={!session.hasNotes}
            onClick={() => onViewNotes(session)}
          >
            Notes
          </button>
        </div>
      </div>
    </div>
  );
}

// Chat tab — embedded terminal with xterm.js + PTY
function ChatTab({
  session,
  state,
}: {
  session: Session | null;
  state: NotateState | null;
}) {
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<XTerm | null>(null);
  const ptyRef = useRef<any>(null);

  useEffect(() => {
    if (!terminalRef.current || !session) return;

    if (xtermRef.current) {
      xtermRef.current.dispose();
      xtermRef.current = null;
    }

    const term = new XTerm({
      theme: {
        background: "#1e1e1e",
        foreground: "#cccccc",
        cursor: "#4fc1ff",
        selectionBackground: "#094771",
      },
      fontFamily: "'SF Mono', Menlo, monospace",
      fontSize: 13,
      cursorBlink: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();

    xtermRef.current = term;

    const initPty = async () => {
      try {
        const pty = spawn("/bin/zsh", ["-l"], {
          cwd: session.path,
          cols: term.cols,
          rows: term.rows,
        });

        ptyRef.current = pty;

        pty.onData((data: Uint8Array) => {
          term.write(data);
        });

        term.onData((data: string) => {
          pty.write(data);
        });

        if (state?.session_id) {
          setTimeout(() => {
            pty.write(`claude --resume ${state.session_id}\n`);
          }, 500);
        }
      } catch (err) {
        term.write(`\r\nFailed to start terminal: ${err}\r\n`);
        console.error("PTY error:", err);
      }
    };

    initPty();

    const resizeObserver = new ResizeObserver(() => {
      fitAddon.fit();
      if (ptyRef.current) {
        ptyRef.current.resize(term.cols, term.rows);
      }
    });
    resizeObserver.observe(terminalRef.current);

    // Listen for menu bar commands
    const handlePtyCommand = (e: Event) => {
      const command = (e as CustomEvent).detail as string;
      if (ptyRef.current) {
        ptyRef.current.write(command + "\n");
      }
    };
    window.addEventListener("pty-command", handlePtyCommand);

    return () => {
      window.removeEventListener("pty-command", handlePtyCommand);
      resizeObserver.disconnect();
      if (ptyRef.current) {
        ptyRef.current.kill();
        ptyRef.current = null;
      }
      term.dispose();
      xtermRef.current = null;
    };
  }, [session, state?.session_id]);

  if (!session) {
    return (
      <div className="terminal-content">
        <p className="empty">Select a session to start chatting.</p>
      </div>
    );
  }

  return <div ref={terminalRef} className="xterm-container" />;
}
