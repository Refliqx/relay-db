<script lang="ts">
  import { DOCS, ISSUES, RELEASES, REPO } from "../lib/links";
  import Logo from "./Logo.svelte";

  const year = new Date().getFullYear();

  const columns = [
    {
      title: "Product",
      links: [
        { label: "Features", href: "#features" },
        { label: "How it works", href: "#how" },
        { label: "Get started", href: "#get-started" }
      ]
    },
    {
      title: "Resources",
      links: [
        { label: "Documentation", href: DOCS },
        { label: "Architecture", href: `${DOCS}/ARCHITECTURE.md` },
        { label: "IPC protocol", href: `${DOCS}/IPC-PROTOCOL.md` }
      ]
    },
    {
      title: "Project",
      links: [
        { label: "GitHub", href: REPO },
        { label: "Releases", href: RELEASES },
        { label: "Issues", href: ISSUES }
      ]
    }
  ];
</script>

<footer class="bg-canvas">
  <div class="shell py-14">
    <div class="grid gap-10 sm:grid-cols-2 lg:grid-cols-[1.6fr_1fr_1fr_1fr]">
      <div>
        <a href="#top" class="flex items-center gap-2" aria-label="RelayDB home">
          <Logo size={18} />
          <span class="text-[15px] font-semibold tracking-tight">
            Relay<span class="text-accent">DB</span>
          </span>
        </a>
        <p class="mt-3 max-w-xs text-[13.5px] leading-relaxed text-muted">
          Seamlessly relay, sanitize and prune live databases into clean local seed
          data.
        </p>
      </div>

      {#each columns as column}
        <div>
          <p class="label">{column.title}</p>
          <ul class="mt-3 space-y-2">
            {#each column.links as link}
              <li>
                <a
                  class="text-[13.5px] text-muted transition-colors hover:text-ink"
                  href={link.href}
                  target={link.href.startsWith("http") ? "_blank" : undefined}
                  rel={link.href.startsWith("http") ? "noreferrer" : undefined}
                >
                  {link.label}
                </a>
              </li>
            {/each}
          </ul>
        </div>
      {/each}
    </div>

    <div
      class="mt-12 flex flex-col gap-2 border-t border-line pt-6 text-[12.5px] text-faint sm:flex-row sm:items-center"
    >
      <p>© {year} RelayDB · MIT licensed</p>
      <p class="sm:ml-auto">Built with Tauri, Rust and Go</p>
    </div>
  </div>
</footer>
