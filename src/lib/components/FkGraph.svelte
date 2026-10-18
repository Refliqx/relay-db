<script lang="ts">
  /**
   * FkGraph: FK relationship visualisation.
   *
   * Design note: this is NOT a force-directed graph. A layered layout is derived
   * from the topological sort result, because the real goal is not "pretty", but
   * answering one technical question: "which table must be INSERTed first?".
   * So arrows are drawn following insert order (parent on the left → child on
   * the right); the column is the distance from the root table.
   *
   * Technical note: colours use the `style` attribute, not presentation
   * attributes (fill="var(--x)"), because `var()` is not guaranteed to work on
   * SVG presentation attributes.
   */
  import type { GraphResult, OrderResult } from "$lib/types/protocol";
  import { formatRows } from "$lib/utils/format";

  interface Props {
    graph: GraphResult;
    order: OrderResult;
    active?: string | null;
    onselect?: (id: string) => void;
  }

  let { graph, order, active = null, onselect }: Props = $props();

  /** Column level = longest distance from a root (a table with no parent). */
  const levels = $derived.by(() => {
    const parents = new Map<string, string[]>();
    for (const node of graph.nodes) parents.set(node.id, []);
    for (const e of graph.edges) {
      if (e.from === e.to) continue;
      parents.get(e.from)?.push(e.to);
    }
    const level = new Map<string, number>();
    const visit = (id: string, seen: Set<string>): number => {
      const cached = level.get(id);
      if (cached !== undefined) return cached;
      if (seen.has(id)) return 0; // cycle: stop the recursion
      seen.add(id);
      const ps = parents.get(id) ?? [];
      const value = ps.length === 0 ? 0 : Math.max(...ps.map((parent) => visit(parent, seen) + 1));
      level.set(id, value);
      return value;
    };
    for (const node of graph.nodes) visit(node.id, new Set());
    return level;
  });

  const NODE_W = 184;
  const NODE_H = 32;
  const GAP_X = 96;
  const GAP_Y = 14;
  const PAD = 16;
  const MONO = "ui-monospace, Menlo, Consolas, monospace";

  const layout = $derived.by(() => {
    const byLevel = new Map<number, string[]>();
    for (const node of graph.nodes) {
      const lv = levels.get(node.id) ?? 0;
      byLevel.set(lv, [...(byLevel.get(lv) ?? []), node.id]);
    }
    const pos = new Map<string, { x: number; y: number }>();
    let maxLevel = 0;
    let maxCount = 0;
    for (const [lv, ids] of byLevel) {
      maxLevel = Math.max(maxLevel, lv);
      maxCount = Math.max(maxCount, ids.length);
      ids
        .sort((a, b) => a.localeCompare(b))
        .forEach((id, i) => {
          pos.set(id, { x: PAD + lv * (NODE_W + GAP_X), y: PAD + i * (NODE_H + GAP_Y) });
        });
    }
    return {
      pos,
      width: PAD * 2 + (maxLevel + 1) * NODE_W + maxLevel * GAP_X,
      height: PAD * 2 + Math.max(1, maxCount) * (NODE_H + GAP_Y),
    };
  });

  const rowsById = $derived(new Map(graph.nodes.map((n) => [n.id, n.estimatedRows])));

  /**
   * The edge path hangs between facing sides: it starts at the parent's right
   * edge and ends at the child's left edge. For "backward" edges (a cycle, where
   * the parent sits to the right of the child) the direction is flipped so the
   * curve does not run into a node.
   */
  function edgePath(parentId: string, childId: string): string {
    const p = layout.pos.get(parentId);
    const c = layout.pos.get(childId);
    if (!p || !c) return "";
    const py = p.y + NODE_H / 2;
    const cy = c.y + NODE_H / 2;
    if (p.x <= c.x) {
      const x1 = p.x + NODE_W;
      const x2 = c.x;
      const mid = (x1 + x2) / 2;
      return `M ${x1} ${py} C ${mid} ${py}, ${mid} ${cy}, ${x2} ${cy}`;
    }
    const x1 = p.x;
    const x2 = c.x + NODE_W;
    const mid = (x1 + x2) / 2;
    return `M ${x1} ${py} C ${mid} ${py}, ${mid} ${cy}, ${x2} ${cy}`;
  }

  const isActive = (id: string) => active === id || active === id.replace(/^.*\./, "");
  const trim = (label: string) => (label.length > 24 ? `${label.slice(0, 23)}…` : label);
</script>

<section class="panel overflow-hidden">
  <header class="panel-head">
    <div class="flex items-center gap-2">
      <h3 class="text-[12.5px] font-semibold text-ink">Foreign key graph</h3>
      <span class="chip tnum">{graph.nodes.length} tables · {graph.edges.length} relationships</span>
    </div>
    <div class="flex items-center gap-1.5 text-[10.5px] text-faint">
      <svg width="22" height="6" aria-hidden="true">
        <line x1="0" y1="3" x2="16" y2="3" style="stroke:var(--line-strong);stroke-width:1.5" />
        <path d="M16 0 L22 3 L16 6 Z" style="fill:var(--line-strong)" />
      </svg>
      <span>parent → child · insert order</span>
    </div>
  </header>

  <div class="max-h-[340px] overflow-auto scroll bg-canvas/40">
    <svg
      width={Math.max(layout.width, 480)}
      height={Math.max(layout.height, 80)}
      class="selectable"
      role="img"
      aria-label="Table relationship graph"
    >
      <defs>
        <marker id="fk-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto">
          <path d="M 0 0 L 10 5 L 0 10 z" style="fill:var(--line-strong)" />
        </marker>
        <marker id="fk-arrow-hot" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="6" markerHeight="6" orient="auto">
          <path d="M 0 0 L 10 5 L 0 10 z" style="fill:var(--accent)" />
        </marker>
      </defs>

      {#each graph.edges as e (e.from + e.to + e.constraint)}
        {@const d = edgePath(e.to, e.from)}
        {@const hot = isActive(e.from) || isActive(e.to)}
        {#if d}
          <path
            d={d}
            style="fill:none;stroke:{hot ? "var(--accent)" : "var(--line-strong)"};stroke-width:{hot
              ? 1.75
              : 1.25}"
            marker-end={hot ? "url(#fk-arrow-hot)" : "url(#fk-arrow)"}
          />
        {/if}
      {/each}

      {#each graph.nodes as node (node.id)}
        {@const p = layout.pos.get(node.id)!}
        {@const on = isActive(node.id)}
        {@const deferred = order.deferred.includes(node.id)}
        <g
          transform={`translate(${p.x} ${p.y})`}
          class="cursor-pointer"
          role="button"
          tabindex="0"
          aria-label={`Table ${node.label}`}
          onclick={() => onselect?.(node.id)}
          onkeydown={(ev) => (ev.key === "Enter" || ev.key === " ") && onselect?.(node.id)}
        >
          <rect
            width={NODE_W}
            height={NODE_H}
            rx="5"
            style="fill:{on ? 'var(--accent)' : 'var(--raised)'};stroke:{on
              ? 'var(--accent)'
              : deferred
                ? 'var(--warn)'
                : 'var(--line)'};{deferred && !on ? 'stroke-dasharray:3 2;' : ''}"
          />
          <text x="10" y="20" font-size="11" style="font-family:{MONO};fill:{on ? 'var(--accent-ink)' : 'var(--ink)'}">
            {trim(node.label)}
          </text>
          <text
            x={NODE_W - 10}
            y="20"
            text-anchor="end"
            font-size="10"
            style="font-family:{MONO};fill:{on ? 'var(--accent-ink)' : 'var(--faint)'};opacity:{on
              ? 0.75
              : 1}"
          >
            {formatRows(rowsById.get(node.id) ?? null)}
          </text>
        </g>
      {/each}
    </svg>
  </div>

  {#if graph.cycles.length > 0}
    <div class="flex items-start gap-2 border-t border-warn/30 bg-warn/[0.08] px-3.5 py-2 text-[11.5px] text-warn">
      <span class="mt-0.5">●</span>
      <span>
        FK cycle detected: {graph.cycles.map((c) => c.join(" ↔ ")).join(" · ")}. A pure insert order is
        not enough; enable <strong class="font-semibold">FK guard</strong> in step 4.
      </span>
    </div>
  {/if}
  {#if order.deferred.length > 0}
    <div class="border-t border-line px-3.5 py-2 text-[11px] text-muted">
      Deferred (inside a cycle): <span class="mono">{order.deferred.join(", ")}</span>
    </div>
  {/if}
</section>
