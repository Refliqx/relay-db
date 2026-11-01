<script lang="ts">
  import { onMount } from "svelte";
  import { Menu, Moon, Sun } from "lucide-svelte";
  import { REPO } from "../lib/links";
  import GithubMark from "./GithubMark.svelte";
  import Logo from "./Logo.svelte";

  let theme = $state<"dark" | "light">("dark");
  let open = $state(false);

  const links = [
    { href: "#features", label: "Features" },
    { href: "#how", label: "How it works" },
    { href: "#security", label: "Security" },
    { href: "#faq", label: "FAQ" }
  ];

  onMount(() => {
    theme = document.documentElement.dataset.theme === "light" ? "light" : "dark";
  });

  function toggleTheme() {
    theme = theme === "dark" ? "light" : "dark";
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("relaydb.theme", theme);
    } catch {
      /* localStorage blocked: the toggle still works for this visit */
    }
  }
</script>

<header class="sticky top-0 z-40 border-b border-line bg-canvas/80 backdrop-blur-md">
  <div class="shell flex h-16 items-center gap-2">
    <a href="#top" class="flex items-center gap-2" aria-label="RelayDB home">
      <Logo size={18} />
      <span class="text-[15px] font-semibold tracking-tight">
        Relay<span class="text-accent">DB</span>
      </span>
    </a>

    <nav class="ml-4 hidden items-center gap-0.5 md:flex" aria-label="Main">
      {#each links as link}
        <a class="nav-link" href={link.href}>{link.label}</a>
      {/each}
    </nav>

    <div class="ml-auto flex items-center gap-1.5">
      <button class="btn-icon" onclick={toggleTheme} aria-label="Switch theme">
        {#if theme === "dark"}<Sun size={16} />{:else}<Moon size={16} />{/if}
      </button>
      <a
        class="btn-icon hidden sm:inline-flex"
        href={REPO}
        target="_blank"
        rel="noreferrer"
        aria-label="RelayDB on GitHub"
      >
        <GithubMark size={16} />
      </a>
      <a class="btn btn-primary btn-sm" href="#get-started">Get started</a>
      <button
        class="btn-icon md:hidden"
        onclick={() => (open = !open)}
        aria-label="Toggle navigation"
        aria-expanded={open}
      >
        <Menu size={17} />
      </button>
    </div>
  </div>

  {#if open}
    <div class="border-t border-line bg-canvas md:hidden">
      <nav class="shell flex flex-col py-2" aria-label="Mobile">
        {#each links as link}
          <a class="nav-link" href={link.href} onclick={() => (open = false)}>{link.label}</a>
        {/each}
        <a class="nav-link" href={REPO} target="_blank" rel="noreferrer">GitHub</a>
      </nav>
    </div>
  {/if}
</header>
