<script lang="ts">
  import { Check } from "lucide-svelte";
  import CodePanel from "./CodePanel.svelte";

  const sql = `-- relaydb seed
-- seed 4f2a9c1e · generated 2026-10-18
SET statement_timeout = 0;

INSERT INTO "public"."users" ("id", "email", "name") VALUES
  (1, 'ada.k@example.net', 'Ada K.'),
  (2, 'grace.h@example.net', 'Grace H.');

INSERT INTO "public"."orders" ("id", "user_id", "total") VALUES
  (1, 1, 129.00),
  (2, 2, 84.50);

-- 24 tables · 38 relationships · 18,400 rows`;

  const points = [
    { lead: "Text, not binary.", body: "A seed is a file a reviewer can diff." },
    { lead: "Foreign-key order.", body: "Imports without disabling constraints." },
    { lead: "Portable.", body: "Pick the dialect at export time." }
  ];
</script>

<section id="output" class="border-b border-line">
  <div class="shell py-20 sm:py-24">
    <div class="grid items-center gap-12 lg:grid-cols-[0.85fr_1.15fr] lg:gap-16">
      <div>
        <p class="kicker">The output</p>
        <h2 class="mt-3 text-[clamp(1.9rem,4vw,2.75rem)] font-semibold">
          A seed file you can read.
        </h2>
        <p class="mt-4 text-[16.5px] leading-relaxed text-muted">
          Plain SQL, one statement per table, ordered by the foreign-key graph.
          Masked values are already in place, so there is nothing to clean up
          before you commit it.
        </p>

        <ul class="mt-6 space-y-2.5 text-[14px] leading-relaxed text-muted">
          {#each points as point}
            <li class="flex gap-2.5">
              <Check size={15} class="mt-0.5 shrink-0 text-ok" />
              <span><span class="text-ink">{point.lead}</span> {point.body}</span>
            </li>
          {/each}
        </ul>
      </div>

      <CodePanel title="seed.sql" meta="48.2 MB" copy={sql}>
        <span class="code-line">
          <span class="code-ln">1</span>
          <span class="code-src"><span class="tok-com">-- relaydb seed</span></span>
        </span>
        <span class="code-line">
          <span class="code-ln">2</span>
          <span class="code-src"><span class="tok-com">-- seed 4f2a9c1e · generated 2026-10-18</span></span>
        </span>
        <span class="code-line">
          <span class="code-ln">3</span>
          <span class="code-src"><span class="tok-key">SET</span> statement_timeout = 0;</span>
        </span>
        <span class="code-line">
          <span class="code-ln">4</span>
          <span class="code-src"> </span>
        </span>
        <span class="code-line">
          <span class="code-ln">5</span>
          <span class="code-src"><span class="tok-key">INSERT INTO</span> "public"."users" ("id", "email", "name") <span class="tok-key">VALUES</span></span>
        </span>
        <span class="code-line">
          <span class="code-ln">6</span>
          <span class="code-src">  (1, <span class="tok-str">'ada.k@example.net'</span>, <span class="tok-str">'Ada K.'</span>),</span>
        </span>
        <span class="code-line">
          <span class="code-ln">7</span>
          <span class="code-src">  (2, <span class="tok-str">'grace.h@example.net'</span>, <span class="tok-str">'Grace H.'</span>);</span>
        </span>
        <span class="code-line">
          <span class="code-ln">8</span>
          <span class="code-src"> </span>
        </span>
        <span class="code-line">
          <span class="code-ln">9</span>
          <span class="code-src"><span class="tok-key">INSERT INTO</span> "public"."orders" ("id", "user_id", "total") <span class="tok-key">VALUES</span></span>
        </span>
        <span class="code-line">
          <span class="code-ln">10</span>
          <span class="code-src">  (1, 1, 129.00),</span>
        </span>
        <span class="code-line">
          <span class="code-ln">11</span>
          <span class="code-src">  (2, 2, 84.50);</span>
        </span>
        <span class="code-line">
          <span class="code-ln">12</span>
          <span class="code-src"> </span>
        </span>
        <span class="code-line">
          <span class="code-ln">13</span>
          <span class="code-src"><span class="tok-com">-- 24 tables · 38 relationships · 18,400 rows</span></span>
        </span>
      </CodePanel>
    </div>
  </div>
</section>
