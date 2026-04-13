import { useEffect, useState, useRef, useCallback } from "react";
import { readDir, exists, writeTextFile, mkdir, remove, stat, readTextFile, watch } from "@tauri-apps/plugin-fs";
import { homeDir } from "@tauri-apps/api/path";
import type { Session, Profile } from "../App";

interface Props {
  selectedSession: Session | null;
  onSelectSession: (session: Session) => void;
  profiles: Profile[];
  onProfilesChanged: () => void;
  onSelectProfile: (name: string, path: string) => void;
}

export default function SessionList({
  selectedSession,
  onSelectSession,
  profiles,
  onProfilesChanged,
  onSelectProfile,
}: Props) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [loading, setLoading] = useState(true);
  const [profilesOpen, setProfilesOpen] = useState(false);
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [showNewProfileInput, setShowNewProfileInput] = useState(false);
  const [newProfileName, setNewProfileName] = useState("");
  const [profileError, setProfileError] = useState("");
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number; session: Session } | null>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [searchResults, setSearchResults] = useState<Set<string> | null>(null);
  const searchTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    loadSessions();
    return () => {
      if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);
    };
  }, []);

  // #4 — Auto-refresh: watch ~/AudioCapture/ for changes
  useEffect(() => {
    let cancelled = false;
    let unwatch: (() => void) | null = null;
    let debounceTimer: ReturnType<typeof setTimeout> | null = null;

    async function startWatching() {
      try {
        const home = await homeDir();
        const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
        const baseDir = `${homePath}/AudioCapture`;
        const dirExists = await exists(baseDir);
        if (!dirExists || cancelled) return;

        const unwatchFn = await watch(baseDir, () => {
          if (cancelled) return;
          if (debounceTimer) clearTimeout(debounceTimer);
          debounceTimer = setTimeout(() => loadSessions(), 500);
        }, { recursive: true });

        if (!cancelled) {
          unwatch = unwatchFn;
        } else {
          unwatchFn();
        }
      } catch (err) {
        console.error("Failed to watch sessions directory:", err);
      }
    }

    startWatching();

    return () => {
      cancelled = true;
      if (unwatch) unwatch();
      if (debounceTimer) clearTimeout(debounceTimer);
    };
  }, []);

  const loadSessions = useCallback(async () => {
    try {
      const home = await homeDir();
      const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
      const baseDir = `${homePath}/AudioCapture`;

      const dirExists = await exists(baseDir);
      if (!dirExists) {
        setLoading(false);
        return;
      }

      const entries = await readDir(baseDir);
      const sessionList: Session[] = [];

      for (const entry of entries) {
        if (!entry.isDirectory) continue;

        const path = `${baseDir}/${entry.name}`;
        const [hasNotes, hasTranscript, hasRecording] = await Promise.all([
          exists(`${path}/notes.md`),
          exists(`${path}/transcript.json`),
          exists(`${path}/recording.wav`),
        ]);

        // #3 — Calculate session size
        let sizeBytes = 0;
        try {
          const files = await readDir(path);
          for (const f of files) {
            if (f.isFile) {
              try {
                const s = await stat(`${path}/${f.name}`);
                sizeBytes += s.size;
              } catch { /* skip files we can't stat */ }
            }
          }
        } catch { /* skip if can't read dir */ }

        sessionList.push({
          name: entry.name,
          path,
          hasNotes,
          hasTranscript,
          hasRecording,
          sizeBytes,
        });
      }

      sessionList.sort((a, b) => b.name.localeCompare(a.name));
      setSessions(sessionList);
    } catch (err) {
      console.error("Failed to load sessions:", err);
    } finally {
      setLoading(false);
    }
  }, []);

  // #6 — Search across sessions
  function handleSearchChange(query: string) {
    setSearchQuery(query);
    if (searchTimeoutRef.current) clearTimeout(searchTimeoutRef.current);

    if (!query.trim()) {
      setSearchResults(null);
      return;
    }

    searchTimeoutRef.current = setTimeout(async () => {
      const matches = new Set<string>();
      const lowerQuery = query.toLowerCase();

      for (const session of sessions) {
        // Search transcript.txt
        try {
          const txtPath = `${session.path}/transcript.txt`;
          if (await exists(txtPath)) {
            const content = await readTextFile(txtPath);
            if (content.toLowerCase().includes(lowerQuery)) {
              matches.add(session.name);
              continue;
            }
          }
        } catch { /* skip */ }

        // Search notes.md
        try {
          const notesPath = `${session.path}/notes.md`;
          if (await exists(notesPath)) {
            const content = await readTextFile(notesPath);
            if (content.toLowerCase().includes(lowerQuery)) {
              matches.add(session.name);
            }
          }
        } catch { /* skip */ }
      }

      setSearchResults(matches);
    }, 300);
  }

  function formatSessionName(name: string): { date: string; label: string | null } {
    const match = name.match(
      /^(\d{4})-(\d{2})-(\d{2})_(\d{2})-(\d{2})-(\d{2})(?:_(.+))?$/
    );
    if (!match) return { date: name, label: null };

    const [, year, month, day, hour, min, , label] = match;
    const date = new Date(
      parseInt(year),
      parseInt(month) - 1,
      parseInt(day),
      parseInt(hour),
      parseInt(min)
    );

    const formatted = date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      hour: "numeric",
      minute: "2-digit",
    });

    return { date: formatted, label: label || null };
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
  }

  function handleProfileClick(profile: Profile) {
    setSelectedProfile(profile.name);
    onSelectProfile(profile.name, profile.path);
  }

  async function handleCreateProfile() {
    if (!newProfileName.trim()) return;

    const safeName = newProfileName.trim().toLowerCase().replace(/[^a-z0-9_-]/g, "-");
    if (!safeName || safeName === "-") {
      setProfileError("Name must contain at least one letter or number");
      return;
    }
    const home = await homeDir();
    const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
    const profileDir = `${homePath}/.notate/profiles`;

    try {
      const dirExists = await exists(profileDir);
      if (!dirExists) {
        await mkdir(profileDir, { recursive: true });
      }

      const path = `${profileDir}/${safeName}.md`;
      const fileExists = await exists(path);
      if (fileExists) {
        setProfileError(`"${safeName}" already exists`);
        return;
      }

      const template = `<!-- Describe what this profile is for -->\nYou are monitoring a conversation. As I feed you new transcript chunks:\n\n1. Maintain notes.md in this directory — update it each time with new information\n2. Extract and organize the most important points\n3. Format with clear markdown sections and bullet points\n4. Include timestamps like [12:30] for when things were said\n5. Preserve everything from previous updates, only add new items\n`;

      await writeTextFile(path, template);
      onProfilesChanged();

      setSelectedProfile(safeName);
      onSelectProfile(safeName, path);
      setShowNewProfileInput(false);
      setNewProfileName("");
      setProfileError("");
    } catch (err) {
      console.error("Failed to create profile:", err);
    }
  }

  function handleContextMenu(e: React.MouseEvent, session: Session) {
    e.preventDefault();
    setContextMenu({ x: e.clientX, y: e.clientY, session });
  }

  async function handleDeleteSession(session: Session) {
    setContextMenu(null);
    try {
      await remove(session.path, { recursive: true });

      // Clean up Claude Code session files
      const home = await homeDir();
      const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
      const encoded = session.path.split("/").join("-").split("_").join("-");
      const claudeDir = `${homePath}/.claude/projects/${encoded}`;
      try {
        await remove(claudeDir, { recursive: true });
      } catch { /* Claude session dir might not exist */ }

      await loadSessions();
    } catch (err) {
      console.error("Failed to delete session:", err);
    }
  }

  // Filter sessions by search
  const filteredSessions = searchResults
    ? sessions.filter((s) => searchResults.has(s.name))
    : sessions;

  if (loading) {
    return (
      <div className="session-list">
        <p className="empty">Loading...</p>
      </div>
    );
  }

  return (
    <div className="session-list">
      {/* Search bar */}
      <div className="search-bar">
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => handleSearchChange(e.target.value)}
          placeholder="Search sessions..."
          className="search-input"
        />
        {searchQuery && (
          <button className="search-clear" onClick={() => { setSearchQuery(""); setSearchResults(null); }}>
            ✕
          </button>
        )}
      </div>

      <div className="session-list-header">
        <h2>Sessions</h2>
        <button
          className="refresh-btn"
          onClick={() => { loadSessions(); onProfilesChanged(); }}
          title="Refresh"
        >
          ↻
        </button>
      </div>

      {filteredSessions.length === 0 ? (
        <div className="session-items">
          <p className="empty">
            {searchQuery ? "No matching sessions." : (
              <>No sessions yet.<br />Run <code>autonote LTW</code> to start.</>
            )}
          </p>
        </div>
      ) : (
        <ul className="session-items">
          {filteredSessions.map((session) => {
            const { date, label } = formatSessionName(session.name);
            return (
              <li
                key={session.name}
                className={selectedSession?.name === session.name ? "selected" : ""}
                onClick={() => onSelectSession(session)}
                onContextMenu={(e) => handleContextMenu(e, session)}
              >
                <div className="session-info-stack">
                  <span className="session-date">
                    {date}
                    {session.sizeBytes > 0 && (
                      <span className="session-size"> · {formatSize(session.sizeBytes)}</span>
                    )}
                  </span>
                  {label && <span className="session-label-text">{label}</span>}
                </div>
                <span className="session-indicators">
                  {session.hasRecording && <span className="badge rec">REC</span>}
                  {session.hasTranscript && <span className="badge txt">TXT</span>}
                  {session.hasNotes && <span className="badge note">NOTE</span>}
                </span>
              </li>
            );
          })}
        </ul>
      )}

      <div
        className="session-list-header profiles-header"
        onClick={() => setProfilesOpen(!profilesOpen)}
      >
        <h2>{profilesOpen ? "▾" : "▸"} Profiles</h2>
      </div>

      {profilesOpen && (
        <ul className="profile-items">
          {showNewProfileInput ? (
            <li className="new-profile-input">
              <form onSubmit={(e) => { e.preventDefault(); handleCreateProfile(); }}>
                <input
                  type="text"
                  value={newProfileName}
                  onChange={(e) => { setNewProfileName(e.target.value); setProfileError(""); }}
                  placeholder="profile-name"
                  className={profileError ? "input-error" : ""}
                  autoFocus
                  onBlur={() => {
                    if (!newProfileName.trim()) {
                      setShowNewProfileInput(false);
                      setProfileError("");
                    }
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Escape") {
                      setShowNewProfileInput(false);
                      setNewProfileName("");
                      setProfileError("");
                    }
                  }}
                />
                {profileError && <span className="input-error-msg">{profileError}</span>}
              </form>
            </li>
          ) : (
            <li className="new-profile-btn" onClick={() => setShowNewProfileInput(true)}>
              <span className="profile-name">+ New Profile</span>
            </li>
          )}
          {profiles.map((profile) => (
            <li
              key={profile.name}
              className={selectedProfile === profile.name ? "selected" : ""}
              onClick={() => handleProfileClick(profile)}
            >
              <span className="profile-entry">
                <span className="profile-name">{profile.name}</span>
                {profile.description && (
                  <span className="profile-desc">: {profile.description}</span>
                )}
              </span>
            </li>
          ))}
          {profiles.length === 0 && (
            <p className="empty">No profiles found.</p>
          )}
        </ul>
      )}

      {/* Context menu */}
      {contextMenu && (
        <>
          <div className="context-menu-overlay" onClick={() => setContextMenu(null)} />
          <div
            className="context-menu"
            style={{ top: contextMenu.y, left: contextMenu.x }}
          >
            <button
              className="context-menu-item danger"
              onClick={() => handleDeleteSession(contextMenu.session)}
            >
              Delete Session
            </button>
          </div>
        </>
      )}
    </div>
  );
}
