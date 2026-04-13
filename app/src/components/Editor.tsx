import { useEffect, useRef, useCallback, useState } from "react";
import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import { Markdown } from "tiptap-markdown";
import Underline from "@tiptap/extension-underline";
import TextAlign from "@tiptap/extension-text-align";
import Highlight from "@tiptap/extension-highlight";
import Color from "@tiptap/extension-color";
import { TextStyle } from "@tiptap/extension-text-style";
// Link is provided by tiptap-markdown — don't import separately
import { Table } from "@tiptap/extension-table";
import TableRow from "@tiptap/extension-table-row";
import TableCell from "@tiptap/extension-table-cell";
import TableHeader from "@tiptap/extension-table-header";
import TaskList from "@tiptap/extension-task-list";
import TaskItem from "@tiptap/extension-task-item";
import Superscript from "@tiptap/extension-superscript";
import Subscript from "@tiptap/extension-subscript";
import Image from "@tiptap/extension-image";
import Placeholder from "@tiptap/extension-placeholder";
import { readTextFile, writeTextFile, exists, watch } from "@tauri-apps/plugin-fs";
import { rename } from "@tauri-apps/plugin-fs";
import { homeDir } from "@tauri-apps/api/path";
import type { EditorTarget } from "../App";

// Inline input that appears in the toolbar for entering URLs, colors, etc.
function ToolbarInput({
  placeholder,
  onSubmit,
  onCancel,
}: {
  placeholder: string;
  onSubmit: (value: string) => void;
  onCancel: () => void;
}) {
  const [value, setValue] = useState("");
  return (
    <form
      className="toolbar-inline-input"
      onSubmit={(e) => { e.preventDefault(); if (value.trim()) onSubmit(value.trim()); }}
    >
      <input
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder={placeholder}
        autoFocus
        onKeyDown={(e) => { if (e.key === "Escape") onCancel(); }}
        onBlur={onCancel}
      />
    </form>
  );
}

interface Props {
  target: EditorTarget | null;
  onProfilesChanged?: () => void;
  onTargetChanged?: (target: EditorTarget) => void;
}

export default function Editor({ target, onProfilesChanged, onTargetChanged }: Props) {
  const saveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const isLoadingRef = useRef(false);
  const currentPathRef = useRef<string | null>(null);
  const [activeInput, setActiveInput] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<"saved" | "saving" | "error" | "idle">("idle");
  const [profileName, setProfileName] = useState("");
  const [profileDesc, setProfileDesc] = useState("");

  const editor = useEditor({
    immediatelyRender: false,
    extensions: [
      StarterKit.configure({ underline: false }),
      Markdown.configure({
        linkify: true,
      }),
      Underline,
      TextStyle,
      Color,
      Highlight.configure({ multicolor: true }),
      TextAlign.configure({ types: ["heading", "paragraph"] }),
      Table.configure({ resizable: true }),
      TableRow,
      TableCell,
      TableHeader,
      TaskList,
      TaskItem.configure({ nested: true }),
      Superscript,
      Subscript,
      Image,
      Placeholder.configure({ placeholder: "Start typing or select a session..." }),
    ],
    content: "",
    onUpdate: ({ editor }) => {
      if (isLoadingRef.current) return;
      if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
      setSaveStatus("saving");
      saveTimeoutRef.current = setTimeout(() => {
        saveNotes((editor.storage as any).markdown.getMarkdown());
      }, 1000);
    },
  });

  const saveNotes = useCallback(async (markdown: string) => {
    if (!currentPathRef.current) return;
    try {
      // For profiles, prepend the description comment
      let content = markdown;
      if (target?.type === "profile" && profileDesc) {
        content = `<!-- ${profileDesc} -->\n${markdown}`;
      }
      await writeTextFile(currentPathRef.current, content);
      setSaveStatus("saved");
    } catch (err) {
      console.error("Failed to save:", err);
      setSaveStatus("error");
    }
  }, [target, profileDesc]);

  useEffect(() => {
    if (!editor || !target) {
      currentPathRef.current = null;
      editor?.commands.setContent("");
      return;
    }

    let cancelled = false;
    currentPathRef.current = target.path;

    async function loadFile() {
      isLoadingRef.current = true;
      try {
        const fileExists = await exists(target!.path);
        if (cancelled) return;
        if (fileExists) {
          let content = await readTextFile(target!.path);
          if (cancelled) return;

          // For profiles, parse out the description comment and show it separately
          if (target!.type === "profile") {
            setProfileName(target!.profileName || "");
            const lines = content.split("\n");
            const firstLine = lines[0]?.trim() || "";
            if (firstLine.startsWith("<!--") && firstLine.endsWith("-->")) {
              setProfileDesc(firstLine.slice(4, -3).trim());
              content = lines.slice(1).join("\n").trimStart();
            } else {
              setProfileDesc("");
            }
          }

          editor!.commands.setContent(content);
        } else {
          editor!.commands.setContent("");
        }
      } catch (err) {
        console.error("Failed to load file:", err);
      } finally {
        isLoadingRef.current = false;
      }
    }

    loadFile();

    // Watch the file for external changes (e.g., Claude Code editing notes.md)
    let unwatch: (() => void) | null = null;

    async function startWatching() {
      try {
        const unwatchFn = await watch(target!.path, async () => {
          if (cancelled) return;
          if (isLoadingRef.current) return;
          if (saveTimeoutRef.current) return;

          isLoadingRef.current = true;
          try {
            const content = await readTextFile(target!.path);
            if (cancelled) return;
            editor!.commands.setContent(content);
            setSaveStatus("saved");
          } catch {
            // File might be mid-write, ignore
          } finally {
            isLoadingRef.current = false;
          }
        }, { delayMs: 500 });
        if (!cancelled) {
          unwatch = unwatchFn;
        } else {
          unwatchFn();
        }
      } catch (err) {
        console.error("Failed to watch file:", err);
      }
    }

    startWatching();

    return () => {
      cancelled = true;
      if (unwatch) unwatch();
      // Clear any pending save
      if (saveTimeoutRef.current) {
        clearTimeout(saveTimeoutRef.current);
        saveTimeoutRef.current = null;
      }
    };
  }, [target, editor]);

  async function handleProfileRename(newName: string) {
    if (!target || target.type !== "profile" || !newName.trim()) return;
    const safeName = newName.trim().toLowerCase().replace(/[^a-z0-9_-]/g, "-");
    if (safeName === target.profileName) return;

    const home = await homeDir();
    const homePath = home.endsWith("/") ? home.slice(0, -1) : home;
    const oldPath = target.path;
    const newPath = `${homePath}/.notate/profiles/${safeName}.md`;

    try {
      const fileExists = await exists(newPath);
      if (fileExists) return; // Don't overwrite existing profile

      await rename(oldPath, newPath);
      setProfileName(safeName);
      currentPathRef.current = newPath;

      onTargetChanged?.({
        type: "profile",
        path: newPath,
        label: `Profile: ${safeName}`,
        profileName: safeName,
      });
      onProfilesChanged?.();
    } catch (err) {
      console.error("Failed to rename profile:", err);
    }
  }

  async function handleDescriptionChange(newDesc: string) {
    setProfileDesc(newDesc);
    // Trigger a save with the updated description
    if (saveTimeoutRef.current) clearTimeout(saveTimeoutRef.current);
    setSaveStatus("saving");
    saveTimeoutRef.current = setTimeout(async () => {
      if (!currentPathRef.current) return;
      try {
        const markdown = (editor!.storage as any).markdown.getMarkdown();
        const content = newDesc ? `<!-- ${newDesc} -->\n${markdown}` : markdown;
        await writeTextFile(currentPathRef.current, content);
        setSaveStatus("saved");
        onProfilesChanged?.();
      } catch (err) {
        console.error("Failed to save:", err);
        setSaveStatus("error");
      }
    }, 1000);
  }

  if (!editor) return null;

  return (
    <div className="editor-container">
      {/* Profile header — editable name and description */}
      {target?.type === "profile" && (
        <div className="profile-edit-header">
          <div className="profile-edit-row">
            <label>Name</label>
            <input
              type="text"
              value={profileName}
              onChange={(e) => setProfileName(e.target.value)}
              onBlur={(e) => handleProfileRename(e.target.value)}
              onKeyDown={(e) => { if (e.key === "Enter") (e.target as HTMLInputElement).blur(); }}
            />
          </div>
          <div className="profile-edit-row">
            <label>Description</label>
            <input
              type="text"
              value={profileDesc}
              onChange={(e) => handleDescriptionChange(e.target.value)}
              placeholder="Brief description of this profile"
            />
          </div>
        </div>
      )}

      <div className="editor-toolbar">
        {/* Undo / Redo */}
        <button onClick={() => editor.chain().focus().undo().run()} title="Undo">↩</button>
        <button onClick={() => editor.chain().focus().redo().run()} title="Redo">↪</button>

        <span className="toolbar-divider" />

        {/* Text formatting */}
        <button
          onClick={() => editor.chain().focus().toggleBold().run()}
          className={editor.isActive("bold") ? "active" : ""}
          title="Bold"
        >B</button>
        <button
          onClick={() => editor.chain().focus().toggleItalic().run()}
          className={editor.isActive("italic") ? "active" : ""}
          title="Italic"
        ><em>I</em></button>
        <button
          onClick={() => editor.chain().focus().toggleUnderline().run()}
          className={editor.isActive("underline") ? "active" : ""}
          title="Underline"
        ><u>U</u></button>
        <button
          onClick={() => editor.chain().focus().toggleStrike().run()}
          className={editor.isActive("strike") ? "active" : ""}
          title="Strikethrough"
        ><s>S</s></button>
        <button
          onClick={() => editor.chain().focus().toggleSuperscript().run()}
          className={editor.isActive("superscript") ? "active" : ""}
          title="Superscript"
        >X<sup>2</sup></button>
        <button
          onClick={() => editor.chain().focus().toggleSubscript().run()}
          className={editor.isActive("subscript") ? "active" : ""}
          title="Subscript"
        >X<sub>2</sub></button>

        <span className="toolbar-divider" />

        {/* Text color & highlight */}
        <button
          onClick={() => setActiveInput(activeInput === "color" ? null : "color")}
          className={activeInput === "color" ? "active" : ""}
          title="Text Color"
        >A<span style={{borderBottom: "2px solid #ff6b6b", paddingBottom: 1}}/></button>
        <button
          onClick={() => setActiveInput(activeInput === "highlight" ? null : "highlight")}
          className={activeInput === "highlight" ? "active" : ""}
          title="Highlight"
        >🖍</button>

        <span className="toolbar-divider" />

        {/* Headings */}
        <button
          onClick={() => editor.chain().focus().toggleHeading({ level: 1 }).run()}
          className={editor.isActive("heading", { level: 1 }) ? "active" : ""}
          title="Heading 1"
        >H1</button>
        <button
          onClick={() => editor.chain().focus().toggleHeading({ level: 2 }).run()}
          className={editor.isActive("heading", { level: 2 }) ? "active" : ""}
          title="Heading 2"
        >H2</button>
        <button
          onClick={() => editor.chain().focus().toggleHeading({ level: 3 }).run()}
          className={editor.isActive("heading", { level: 3 }) ? "active" : ""}
          title="Heading 3"
        >H3</button>

        <span className="toolbar-divider" />

        {/* Lists */}
        <button
          onClick={() => editor.chain().focus().toggleBulletList().run()}
          className={editor.isActive("bulletList") ? "active" : ""}
          title="Bullet List"
        >•</button>
        <button
          onClick={() => editor.chain().focus().toggleOrderedList().run()}
          className={editor.isActive("orderedList") ? "active" : ""}
          title="Numbered List"
        >1.</button>
        <button
          onClick={() => editor.chain().focus().toggleTaskList().run()}
          className={editor.isActive("taskList") ? "active" : ""}
          title="Task List"
        >☑</button>

        <span className="toolbar-divider" />

        {/* Alignment */}
        <button
          onClick={() => editor.chain().focus().setTextAlign("left").run()}
          className={editor.isActive({ textAlign: "left" }) ? "active" : ""}
          title="Align Left"
        >⫷</button>
        <button
          onClick={() => editor.chain().focus().setTextAlign("center").run()}
          className={editor.isActive({ textAlign: "center" }) ? "active" : ""}
          title="Align Center"
        >≡</button>
        <button
          onClick={() => editor.chain().focus().setTextAlign("right").run()}
          className={editor.isActive({ textAlign: "right" }) ? "active" : ""}
          title="Align Right"
        >⫸</button>

        <span className="toolbar-divider" />

        {/* Insert */}
        <button
          onClick={() => setActiveInput(activeInput === "link" ? null : "link")}
          className={activeInput === "link" ? "active" : ""}
          title="Insert Link"
        >🔗</button>
        <button
          onClick={() => setActiveInput(activeInput === "image" ? null : "image")}
          className={activeInput === "image" ? "active" : ""}
          title="Insert Image"
        >🖼</button>
        <button
          onClick={() => editor.chain().focus().insertTable({ rows: 3, cols: 3 }).run()}
          title="Insert Table"
        >⊞</button>
        <button
          onClick={() => editor.chain().focus().toggleCodeBlock().run()}
          className={editor.isActive("codeBlock") ? "active" : ""}
          title="Code Block"
        >{"</>"}</button>
        <button
          onClick={() => editor.chain().focus().toggleBlockquote().run()}
          className={editor.isActive("blockquote") ? "active" : ""}
          title="Blockquote"
        >❝</button>
        <button
          onClick={() => editor.chain().focus().setHorizontalRule().run()}
          title="Horizontal Rule"
        >—</button>

        <span className="toolbar-divider" />

        <button
          onClick={() => window.print()}
          title="Export to PDF"
        >PDF</button>

        <div className="toolbar-spacer" />

        {saveStatus === "saving" && <span className="save-status saving">Saving...</span>}
        {saveStatus === "saved" && <span className="save-status saved">Saved</span>}
        {saveStatus === "error" && <span className="save-status error">Save failed</span>}

        <span className="session-label">
          {target ? target.label : "No file selected"}
        </span>
      </div>

      {/* Inline inputs for link, image, color, highlight */}
      {activeInput === "link" && (
        <ToolbarInput
          placeholder="Enter URL (https://...)"
          onSubmit={(url) => { editor.chain().focus().setLink({ href: url }).run(); setActiveInput(null); }}
          onCancel={() => setActiveInput(null)}
        />
      )}
      {activeInput === "image" && (
        <ToolbarInput
          placeholder="Enter image URL"
          onSubmit={(url) => { editor.chain().focus().setImage({ src: url }).run(); setActiveInput(null); }}
          onCancel={() => setActiveInput(null)}
        />
      )}
      {activeInput === "color" && (
        <ToolbarInput
          placeholder="Color (e.g., red, #ff0000)"
          onSubmit={(color) => { editor.chain().focus().setColor(color).run(); setActiveInput(null); }}
          onCancel={() => setActiveInput(null)}
        />
      )}
      {activeInput === "highlight" && (
        <ToolbarInput
          placeholder="Highlight color (e.g., yellow)"
          onSubmit={(color) => { editor.chain().focus().toggleHighlight({ color }).run(); setActiveInput(null); }}
          onCancel={() => setActiveInput(null)}
        />
      )}

      <EditorContent editor={editor} className="editor-content" />
    </div>
  );
}
