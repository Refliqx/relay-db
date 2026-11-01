<script lang="ts">
  import Comparison from "./components/Comparison.svelte";
  import CtaBand from "./components/CtaBand.svelte";
  import DeepDive from "./components/DeepDive.svelte";
  import Drivers from "./components/Drivers.svelte";
  import Faq from "./components/Faq.svelte";
  import Features from "./components/Features.svelte";
  import Footer from "./components/Footer.svelte";
  import Hero from "./components/Hero.svelte";
  import HowItWorks from "./components/HowItWorks.svelte";
  import Nav from "./components/Nav.svelte";
  import Output from "./components/Output.svelte";
  import Problem from "./components/Problem.svelte";
  import Security from "./components/Security.svelte";
  import Stats from "./components/Stats.svelte";
</script>

<a class="skip-link" href="#main">Skip to content</a>

<Nav />

<main id="main">
  <Hero />
  <Drivers />
  <Problem />
  <Stats />
  <Features />
  <HowItWorks />
  <Output />
  <DeepDive />
  <Comparison />
  <Security />
  <Faq />
  <CtaBand />
</main>

<Footer />
