import { useState, useEffect, useCallback } from "react";
import { Group, Panel, Separator } from "react-resizable-panels";
import { readDir, exists, readTextFile } from "@tauri-apps/plugin-fs";
import { homeDir } from "@tauri-apps/api/path";
import SessionList from "./components/SessionList";
import Editor from "./components/Editor";
import Terminal from "./components/Terminal";
import "./App.css";

export interface Session {
  name: string;
  path: string;
  hasNotes: boolean;
  hasTranscript: boolean;
  hasRecording: boolean;
  sizeBytes: number;
}

export interface EditorTarget {
  type: "session" | "profile";
  path: string;
  label: string;
  profileName?: string;  // Only set for profiles — the filename without .md
}

export interface Profile {
  name: string;
  description: string;
  path: string;
}

function App() {
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [editorTarget, setEditorTarget] = useState<EditorTarget | null>(null);
  const [profiles, setProfiles] = useState<Profile[]>([]);

  const loadProfiles = useCallback(async () => {
    try {
      const home = await homeDir();
      const profileDir = `${home}/.notate/profiles`;

      const dirExists = await exists(profileDir);
      if (!dirExists) return;

      const entries = await readDir(profileDir);
      const profileList: Profile[] = [];

      for (const entry of entries) {
        if (!entry.name.endsWith(".md")) continue;

        const path = `${profileDir}/${entry.name}`;
        const name = entry.name.replace(".md", "");

        let description = "";
        try {
          const content = await readTextFile(path);
          const firstLine = content.split("\n")[0].trim();
          if (firstLine.startsWith("<!--") && firstLine.endsWith("-->")) {
            description = firstLine.slice(4, -3).trim();
          }
        } catch (err) {
          console.error(`Failed to read profile ${name}:`, err);
        }

        profileList.push({ name, description, path });
      }

      profileList.sort((a, b) => a.name.localeCompare(b.name));
      setProfiles(profileList);
    } catch (err) {
      console.error("Failed to load profiles:", err);
    }
  }, []);

  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

  function handleSelectSession(session: Session) {
    setSelectedSession(session);
    setEditorTarget({
      type: "session",
      path: `${session.path}/notes.md`,
      label: session.name,
    });
  }

  function handleSelectProfile(name: string, path: string) {
    setSelectedSession(null);
    setEditorTarget({
      type: "profile",
      path,
      label: `Profile: ${name}`,
      profileName: name,
    });
  }

  function handleViewTranscript(session: Session) {
    setEditorTarget({
      type: "session",
      path: `${session.path}/transcript.txt`,
      label: `${session.name} — transcript`,
    });
  }

  function handleViewJSON(session: Session) {
    setEditorTarget({
      type: "session",
      path: `${session.path}/transcript.json`,
      label: `${session.name} — JSON`,
    });
  }

  function handleViewNotes(session: Session) {
    setEditorTarget({
      type: "session",
      path: `${session.path}/notes.md`,
      label: session.name,
    });
  }

  return (
    <div className="app">
      <Group orientation="horizontal">
        <Panel defaultSize={25} minSize={15} collapsible collapsedSize={0}>
          <SessionList
            selectedSession={selectedSession}
            onSelectSession={handleSelectSession}
            profiles={profiles}
            onProfilesChanged={loadProfiles}
            onSelectProfile={handleSelectProfile}
          />
        </Panel>

        <Separator className="resize-handle" />

        <Panel defaultSize={50} minSize={20}>
          <Editor
            target={editorTarget}
            onProfilesChanged={loadProfiles}
            onTargetChanged={setEditorTarget}
          />
        </Panel>

        <Separator className="resize-handle" />

        <Panel defaultSize={25} minSize={10} collapsible collapsedSize={0}>
          <Terminal
            session={selectedSession}
            profiles={profiles}
            onViewTranscript={handleViewTranscript}
            onViewJSON={handleViewJSON}
            onViewNotes={handleViewNotes}
          />
        </Panel>
      </Group>
    </div>
  );
}

export default App;