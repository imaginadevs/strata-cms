/**
 * Workflow create/edit — n8n-style React Flow canvas + config panel.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Background,
  BackgroundVariant,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  addEdge,
  useEdgesState,
  useNodesState,
  useReactFlow,
  type Connection,
  type Edge,
  type OnSelectionChangeParams,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { ArrowLeft, Mail, Plus, Zap } from "lucide-react";
import { get, patch, post } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import type { Workflow, WorkflowCatalog } from "@/types";
import { nodeTypes, type WfNode } from "./WorkflowNodes";
import {
  WorkflowConfigPanel,
  WorkflowMetaBar,
} from "./WorkflowConfigPanel";
import {
  TRIGGER_NODE_ID,
  actionSummary,
  draftToBody,
  emptyDraft,
  newActionDraft,
  triggerSummary,
  workflowToDraft,
  type WorkflowDraft,
} from "./model";

const TRIGGER_X = 80;
const ACTION_X0 = 380;
const ACTION_GAP = 280;
const NODE_Y = 160;

function buildGraph(draft: WorkflowDraft): { nodes: WfNode[]; edges: Edge[] } {
  const triggerTitle =
    draft.trigger.type === "schedule" ? "Schedule" : "Field changed";

  const nodes: WfNode[] = [
    {
      id: TRIGGER_NODE_ID,
      type: "wf",
      position: { x: TRIGGER_X, y: NODE_Y },
      deletable: false,
      data: {
        kind: "trigger",
        title: triggerTitle,
        subtitle: triggerSummary(draft.trigger),
      },
    },
  ];

  draft.actions.forEach((a, i) => {
    nodes.push({
      id: a.nodeId,
      type: "wf",
      position: { x: ACTION_X0 + i * ACTION_GAP, y: NODE_Y },
      deletable: draft.actions.length > 1,
      data: {
        kind: "action",
        title: "Email with PDF",
        subtitle: actionSummary(a),
      },
    });
  });

  const edges: Edge[] = [];
  const chain = [TRIGGER_NODE_ID, ...draft.actions.map((a) => a.nodeId)];
  for (let i = 0; i < chain.length - 1; i++) {
    edges.push({
      id: `e-${chain[i]}-${chain[i + 1]}`,
      source: chain[i],
      target: chain[i + 1],
      animated: true,
      deletable: true,
      style: { stroke: "#6366f1", strokeWidth: 2 },
    });
  }

  return { nodes, edges };
}

/** Stable signature — only rebuild graph when structure/labels change. */
function draftGraphKey(draft: WorkflowDraft): string {
  return [
    draft.trigger.type,
    triggerSummary(draft.trigger),
    ...draft.actions.map((a) => `${a.nodeId}:${actionSummary(a)}`),
  ].join("|");
}

function EditorCanvas({
  draft,
  selectedId,
  onSelect,
  onDraftChange,
  catalog,
}: {
  draft: WorkflowDraft;
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onDraftChange: (d: WorkflowDraft) => void;
  catalog?: WorkflowCatalog;
}) {
  const { fitView } = useReactFlow();
  const didFit = useRef(false);
  const selectedIdRef = useRef(selectedId);
  selectedIdRef.current = selectedId;

  const graphKey = draftGraphKey(draft);
  const initial = useMemo(() => buildGraph(draft), []); // eslint-disable-line react-hooks/exhaustive-deps -- seed once
  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);

  // Sync labels / node set when draft structure changes — do NOT overwrite
  // selection from selectedId here in a way that loops with onSelectionChange.
  useEffect(() => {
    const g = buildGraph(draft);
    setNodes((prev) => {
      const pos = new Map(prev.map((n) => [n.id, n.position]));
      const selected = new Map(prev.map((n) => [n.id, !!n.selected]));
      return g.nodes.map((n) => ({
        ...n,
        position: pos.get(n.id) ?? n.position,
        selected: selected.get(n.id) ?? n.id === selectedIdRef.current,
      }));
    });
    setEdges(g.edges);
  }, [graphKey, draft, setNodes, setEdges]);

  // Palette / panel-driven selection → reflect on canvas nodes (no empty clears).
  useEffect(() => {
    if (!selectedId) return;
    setNodes((prev) => {
      let changed = false;
      const next = prev.map((n) => {
        const sel = n.id === selectedId;
        if (n.selected !== sel) changed = true;
        return n.selected === sel ? n : { ...n, selected: sel };
      });
      return changed ? next : prev;
    });
  }, [selectedId, setNodes]);

  const onConnect = useCallback(
    (connection: Connection) => {
      const { source, target } = connection;
      if (!source || !target) return;
      // Trigger is start-only — never accept incoming links.
      if (target === TRIGGER_NODE_ID) return;

      setEdges((eds) =>
        addEdge(
          {
            ...connection,
            animated: true,
            deletable: true,
            style: { stroke: "#6366f1", strokeWidth: 2 },
          },
          eds,
        ),
      );

      const actions = [...draft.actions];
      const targetIdx = actions.findIndex((a) => a.nodeId === target);
      if (targetIdx < 0) return;

      const [moved] = actions.splice(targetIdx, 1);
      if (source === TRIGGER_NODE_ID) {
        actions.unshift(moved);
      } else {
        const sourceIdx = actions.findIndex((a) => a.nodeId === source);
        if (sourceIdx < 0) {
          actions.push(moved);
        } else {
          actions.splice(sourceIdx + 1, 0, moved);
        }
      }
      onDraftChange({ ...draft, actions });
    },
    [draft, onDraftChange, setEdges],
  );

  const isValidConnection = useCallback((connection: Connection | Edge) => {
    if (connection.target === TRIGGER_NODE_ID) return false;
    if (connection.source === connection.target) return false;
    return true;
  }, []);

  /** Delete edge → drop the target action from the flow (keep ≥1 action). */
  const onEdgesDelete = useCallback(
    (deleted: Edge[]) => {
      const removeIds = new Set(
        deleted
          .map((e) => e.target)
          .filter((t) => t && t !== TRIGGER_NODE_ID),
      );
      if (removeIds.size === 0) return;
      const next = draft.actions.filter((a) => !removeIds.has(a.nodeId));
      if (next.length === 0) return;
      onDraftChange({ ...draft, actions: next });
      if (selectedIdRef.current && removeIds.has(selectedIdRef.current)) {
        onSelect(TRIGGER_NODE_ID);
      }
    },
    [draft, onDraftChange, onSelect],
  );

  /** Delete action node(s); trigger is never deletable. */
  const onNodesDelete = useCallback(
    (deleted: WfNode[]) => {
      const removeIds = new Set(
        deleted
          .filter((n) => n.id !== TRIGGER_NODE_ID)
          .map((n) => n.id),
      );
      if (removeIds.size === 0) return;
      const next = draft.actions.filter((a) => !removeIds.has(a.nodeId));
      if (next.length === 0) return;
      onDraftChange({ ...draft, actions: next });
      if (selectedIdRef.current && removeIds.has(selectedIdRef.current)) {
        onSelect(TRIGGER_NODE_ID);
      }
    },
    [draft, onDraftChange, onSelect],
  );

  // Sticky selection: RF often emits [] during resize/fitView — ignore clears
  // so the aside doesn't mount/unmount in a loop.
  const onSelectionChange = useCallback(
    ({ nodes: sel }: OnSelectionChangeParams) => {
      const next = sel[0]?.id;
      if (!next) return;
      if (next === selectedIdRef.current) return;
      onSelect(next);
    },
    [onSelect],
  );

  const onInit = useCallback(() => {
    if (didFit.current) return;
    didFit.current = true;
    // Defer until layout has settled (panel already in the tree).
    requestAnimationFrame(() => {
      fitView({ padding: 0.35, duration: 200 });
    });
  }, [fitView]);

  function addAction() {
    const action = newActionDraft();
    onDraftChange({
      ...draft,
      actions: [...draft.actions, action],
    });
    onSelect(action.nodeId);
  }

  return (
    <div className="relative h-full min-h-[420px] w-full">
      <div className="absolute left-3 top-3 z-10 flex w-44 flex-col gap-2 rounded-2xl border border-slate-200/80 bg-white/95 p-2 shadow-lg backdrop-blur dark:border-white/10 dark:bg-slate-900/95">
        <p className="px-1 text-[10px] font-semibold uppercase tracking-widest text-slate-400">
          Add node
        </p>
        <button
          type="button"
          onClick={() => onSelect(TRIGGER_NODE_ID)}
          className="flex items-center gap-2 rounded-xl border border-amber-200/80 bg-amber-50 px-2.5 py-2 text-left text-xs font-medium text-amber-900 transition hover:bg-amber-100 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100"
        >
          <Zap size={14} />
          Trigger
        </button>
        <button
          type="button"
          onClick={addAction}
          className="flex items-center gap-2 rounded-xl border border-brand-200/80 bg-brand-50 px-2.5 py-2 text-left text-xs font-medium text-brand-900 transition hover:bg-brand-100 dark:border-brand-500/30 dark:bg-brand-500/10 dark:text-brand-100"
        >
          <Mail size={14} />
          Email + PDF
        </button>
        {(catalog?.actions?.length ?? 0) > 1 && (
          <p className="px-1 text-[10px] text-slate-400">
            More action types appear here as they ship.
          </p>
        )}
        <button
          type="button"
          onClick={addAction}
          className="flex items-center justify-center gap-1 rounded-xl border border-dashed border-slate-200 px-2 py-1.5 text-[11px] font-medium text-slate-500 hover:bg-slate-50 dark:border-white/10 dark:hover:bg-white/5"
        >
          <Plus size={12} />
          Add action
        </button>
      </div>

      <ReactFlow<WfNode>
        nodes={nodes}
        edges={edges}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onConnect={onConnect}
        onEdgesDelete={onEdgesDelete}
        onNodesDelete={onNodesDelete}
        isValidConnection={isValidConnection}
        onSelectionChange={onSelectionChange}
        onInit={onInit}
        nodeTypes={nodeTypes}
        minZoom={0.4}
        maxZoom={1.5}
        deleteKeyCode={["Backspace", "Delete"]}
        proOptions={{ hideAttribution: true }}
        className="bg-slate-50/80 dark:bg-slate-950/40"
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={18}
          size={1}
          color="currentColor"
          className="text-slate-300 dark:text-slate-700"
        />
        <Controls className="!overflow-hidden !rounded-xl !border-slate-200 !shadow-md dark:!border-white/10 dark:!bg-slate-900" />
        <MiniMap
          className="!overflow-hidden !rounded-xl !border-slate-200 !shadow-md dark:!border-white/10 dark:!bg-slate-900"
          maskColor="rgb(15 23 42 / 0.15)"
          nodeColor={(n) =>
            (n.data as { kind?: string })?.kind === "trigger"
              ? "#f59e0b"
              : "#4f46e5"
          }
        />
      </ReactFlow>
    </div>
  );
}

function WorkflowEditorInner() {
  const params = useParams({ strict: false }) as { id?: string };
  const isNew = !params.id || params.id === "new";
  const id = isNew ? undefined : params.id;
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [draft, setDraft] = useState<WorkflowDraft>(() => emptyDraft());
  const [selectedId, setSelectedId] = useState<string | null>(TRIGGER_NODE_ID);
  const [error, setError] = useState<string | null>(null);
  const [hydrated, setHydrated] = useState(isNew);

  const { data: catalog } = useQuery({
    queryKey: ["workflows", "catalog"],
    queryFn: () => get<WorkflowCatalog>("/workflows/catalog"),
    staleTime: 60_000,
  });

  const { data: existing, isLoading } = useQuery({
    queryKey: ["workflows", id],
    queryFn: () => get<Workflow>(`/workflows/${id}`),
    enabled: !!id,
  });

  useEffect(() => {
    if (existing) {
      setDraft(workflowToDraft(existing));
      setSelectedId(TRIGGER_NODE_ID);
      setHydrated(true);
    }
  }, [existing]);

  const saveMut = useMutation({
    mutationFn: async () => {
      const body = draftToBody(draft);
      if (!body.name) throw new Error("Name is required");
      if (isNew) return post<Workflow>("/workflows", body);
      return patch<Workflow>(`/workflows/${id}`, body);
    },
    onSuccess: (wf) => {
      queryClient.invalidateQueries({ queryKey: ["workflows"] });
      queryClient.setQueryData(["workflows", wf.id], wf);
      navigate({ to: "/workflows" });
    },
    onError: (err: Error) => setError(err.message),
  });

  if (id && (isLoading || !hydrated)) {
    return (
      <div className="py-16 text-center text-sm text-slate-400">Loading…</div>
    );
  }

  return (
    <div className="-m-4 flex h-[calc(100dvh-var(--header-height)-5.5rem)] flex-col sm:-m-6 lg:-m-8">
      <div className="flex shrink-0 flex-wrap items-center gap-3 border-b border-slate-200/80 bg-white/70 px-4 py-3 backdrop-blur dark:border-white/10 dark:bg-slate-900/60">
        <Link
          to="/workflows"
          className="rounded-lg p-1.5 text-slate-400 transition hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-white/5 dark:hover:text-slate-200"
        >
          <ArrowLeft size={18} />
        </Link>
        <div className="min-w-0 flex-1">
          <WorkflowMetaBar draft={draft} onChange={setDraft} />
        </div>
        <Button
          type="button"
          variant="ghost"
          onClick={() => navigate({ to: "/workflows" })}
        >
          Cancel
        </Button>
        <Button
          type="button"
          disabled={saveMut.isPending}
          onClick={() => {
            setError(null);
            saveMut.mutate();
          }}
        >
          {saveMut.isPending
            ? "Saving…"
            : isNew
              ? "Create workflow"
              : "Save changes"}
        </Button>
      </div>

      {error && (
        <p className="shrink-0 border-b border-red-200 bg-red-50 px-4 py-2 text-sm text-red-600 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-400">
          {error}
        </p>
      )}

      {/* Always reserve the aside column so canvas width doesn't thrash. */}
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1">
          <EditorCanvas
            draft={draft}
            selectedId={selectedId}
            onSelect={setSelectedId}
            onDraftChange={(d) => {
              setDraft(d);
              setError(null);
            }}
            catalog={catalog}
          />
        </div>
        <div className="hidden w-full max-w-md shrink-0 md:block">
          <WorkflowConfigPanel
            draft={draft}
            selectedId={selectedId ?? TRIGGER_NODE_ID}
            catalog={catalog}
            onClose={() => setSelectedId(TRIGGER_NODE_ID)}
            onChangeDraft={(d) => {
              setDraft(d);
              setError(null);
              if (
                selectedId &&
                selectedId !== TRIGGER_NODE_ID &&
                !d.actions.some((a) => a.nodeId === selectedId)
              ) {
                setSelectedId(TRIGGER_NODE_ID);
              }
            }}
          />
        </div>
      </div>

      <div className="max-h-[45%] shrink-0 border-t border-slate-200 md:hidden dark:border-white/10">
        <WorkflowConfigPanel
          draft={draft}
          selectedId={selectedId ?? TRIGGER_NODE_ID}
          catalog={catalog}
          onClose={() => setSelectedId(TRIGGER_NODE_ID)}
          onChangeDraft={setDraft}
        />
      </div>
    </div>
  );
}

export default function WorkflowEditorPage() {
  return (
    <ReactFlowProvider>
      <WorkflowEditorInner />
    </ReactFlowProvider>
  );
}
