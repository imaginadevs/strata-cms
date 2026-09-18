/**
 * TipTap-based HTML rich text editor styled to match CMS form inputs.
 * Value is HTML string (or null when empty).
 */
import { forwardRef, useEffect } from "react";
import { useEditor, EditorContent } from "@tiptap/react";
import StarterKit from "@tiptap/starter-kit";
import Link from "@tiptap/extension-link";
import Underline from "@tiptap/extension-underline";
import Placeholder from "@tiptap/extension-placeholder";
import {
  Bold,
  Italic,
  Underline as UnderlineIcon,
  Strikethrough,
  Heading2,
  Heading3,
  List,
  ListOrdered,
  Quote,
  Code,
  Link2,
  Undo2,
  Redo2,
  RemoveFormatting,
} from "lucide-react";
import { cn } from "@/lib/utils";

export interface RichTextEditorProps {
  value: string;
  onChange: (html: string | null) => void;
  placeholder?: string;
  error?: boolean;
  className?: string;
  disabled?: boolean;
}

function ToolbarButton({
  onClick,
  active,
  disabled,
  title,
  children,
}: {
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      title={title}
      disabled={disabled}
      onMouseDown={(e) => e.preventDefault()}
      onClick={onClick}
      className={cn(
        "flex h-8 w-8 items-center justify-center rounded-lg text-slate-500 transition",
        "hover:bg-slate-100 hover:text-slate-800 dark:text-slate-400 dark:hover:bg-white/10 dark:hover:text-slate-100",
        "disabled:pointer-events-none disabled:opacity-40",
        active && "bg-brand-50 text-brand-700 dark:bg-brand-500/20 dark:text-brand-300",
      )}
    >
      {children}
    </button>
  );
}

function ToolbarDivider() {
  return <span className="mx-0.5 h-5 w-px shrink-0 bg-slate-200 dark:bg-white/10" />;
}

export const RichTextEditor = forwardRef<HTMLDivElement, RichTextEditorProps>(
  ({ value, onChange, placeholder, error, className, disabled }, ref) => {
    const editor = useEditor({
      extensions: [
        StarterKit.configure({
          heading: { levels: [2, 3] },
        }),
        Underline,
        Link.configure({
          openOnClick: false,
          HTMLAttributes: {
            class: "text-brand-600 underline underline-offset-2 dark:text-brand-300",
          },
        }),
        Placeholder.configure({
          placeholder: placeholder ?? "Write something…",
        }),
      ],
      content: value || "",
      editable: !disabled,
      // Avoid SSR hydration mismatch warnings under Vite.
      immediatelyRender: false,
      editorProps: {
        attributes: {
          class: "rich-text-editor min-h-[140px] px-3 py-2.5 outline-none",
        },
      },
      onUpdate: ({ editor: ed }) => {
        const html = ed.getHTML();
        onChange(ed.isEmpty || html === "<p></p>" ? null : html);
      },
    });

    // Sync external value changes (e.g. form reset / language switch) without
    // fighting the user's caret while typing.
    useEffect(() => {
      if (!editor) return;
      const next = value || "";
      const current = editor.getHTML();
      if (next === current) return;
      if ((next === "" || next === "<p></p>") && editor.isEmpty) return;
      editor.commands.setContent(next, { emitUpdate: false });
    }, [editor, value]);

    useEffect(() => {
      if (!editor) return;
      editor.setEditable(!disabled);
    }, [editor, disabled]);

    function setLink() {
      if (!editor) return;
      const prev = editor.getAttributes("link").href as string | undefined;
      const url = window.prompt("Link URL", prev ?? "https://");
      if (url === null) return;
      if (url === "") {
        editor.chain().focus().extendMarkRange("link").unsetLink().run();
        return;
      }
      editor.chain().focus().extendMarkRange("link").setLink({ href: url }).run();
    }

    return (
      <div
        ref={ref}
        className={cn(
          "overflow-hidden rounded-lg border bg-white dark:bg-white/5",
          error
            ? "border-red-300 focus-within:border-red-400 focus-within:ring-2 focus-within:ring-red-100"
            : "border-slate-200 dark:border-white/10 focus-within:border-brand-400 dark:focus-within:border-brand-500/60 focus-within:ring-2 focus-within:ring-brand-100 dark:focus-within:ring-brand-500/20",
          className,
        )}
      >
        <div className="flex flex-wrap items-center gap-0.5 border-b border-slate-100 dark:border-white/10 bg-slate-50/80 dark:bg-white/[0.03] px-1.5 py-1">
          <ToolbarButton
            title="Bold"
            active={editor?.isActive("bold")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleBold().run()}
          >
            <Bold size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Italic"
            active={editor?.isActive("italic")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleItalic().run()}
          >
            <Italic size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Underline"
            active={editor?.isActive("underline")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleUnderline().run()}
          >
            <UnderlineIcon size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Strikethrough"
            active={editor?.isActive("strike")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleStrike().run()}
          >
            <Strikethrough size={14} />
          </ToolbarButton>
          <ToolbarDivider />
          <ToolbarButton
            title="Heading 2"
            active={editor?.isActive("heading", { level: 2 })}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleHeading({ level: 2 }).run()}
          >
            <Heading2 size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Heading 3"
            active={editor?.isActive("heading", { level: 3 })}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleHeading({ level: 3 }).run()}
          >
            <Heading3 size={14} />
          </ToolbarButton>
          <ToolbarDivider />
          <ToolbarButton
            title="Bullet list"
            active={editor?.isActive("bulletList")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleBulletList().run()}
          >
            <List size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Numbered list"
            active={editor?.isActive("orderedList")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleOrderedList().run()}
          >
            <ListOrdered size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Quote"
            active={editor?.isActive("blockquote")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleBlockquote().run()}
          >
            <Quote size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Code block"
            active={editor?.isActive("codeBlock")}
            disabled={!editor}
            onClick={() => editor?.chain().focus().toggleCodeBlock().run()}
          >
            <Code size={14} />
          </ToolbarButton>
          <ToolbarDivider />
          <ToolbarButton
            title="Link"
            active={editor?.isActive("link")}
            disabled={!editor}
            onClick={setLink}
          >
            <Link2 size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Clear formatting"
            disabled={!editor}
            onClick={() => editor?.chain().focus().unsetAllMarks().clearNodes().run()}
          >
            <RemoveFormatting size={14} />
          </ToolbarButton>
          <ToolbarDivider />
          <ToolbarButton
            title="Undo"
            disabled={!editor?.can().undo()}
            onClick={() => editor?.chain().focus().undo().run()}
          >
            <Undo2 size={14} />
          </ToolbarButton>
          <ToolbarButton
            title="Redo"
            disabled={!editor?.can().redo()}
            onClick={() => editor?.chain().focus().redo().run()}
          >
            <Redo2 size={14} />
          </ToolbarButton>
        </div>

        <EditorContent editor={editor} />
      </div>
    );
  },
);
RichTextEditor.displayName = "RichTextEditor";
