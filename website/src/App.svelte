<script lang="ts">
  import CtaBand from "./components/CtaBand.svelte";
  import DeepDive from "./components/DeepDive.svelte";
  import Drivers from "./components/Drivers.svelte";
  import Faq from "./components/Faq.svelte";
  import Features from "./components/Features.svelte";
  import Footer from "./components/Footer.svelte";
  import Hero from "./components/Hero.svelte";
  import HowItWorks from "./components/HowItWorks.svelte";
  import Nav from "./components/Nav.svelte";
  import Security from "./components/Security.svelte";
</script>

<a class="skip-link" href="#main">Skip to content</a>

<Nav />

<main id="main">
  <Hero />
  <Drivers />
  <Features />
  <HowItWorks />
  <DeepDive />
  <Security />
  <Faq />
  <CtaBand />
</main>

<Footer />
