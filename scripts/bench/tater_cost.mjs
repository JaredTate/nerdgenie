// Project the Claude Opus 4.8 API dollar cost of a run from its token counts.
//
// Prices are Anthropic Opus 4.8 first-party rates as of 2026-09-03, per million
// tokens: input $5.00, output $25.00, cache read $0.50 (0.1x input); the
// 5-minute cache-write rate is $6.25 (1.25x input) and is not used by this
// projection, which bills first reads at the input rate.
//
// Usage: node tater_cost.mjs <total_input> <uncached_input> <total_output>
//   uncached_input is the daemon's "prompt tokens read" (the first-read part);
//   cached = total_input - uncached_input.
// Prints one JSON object with both projections, rounded to whole cents.

const [ti, un, out] = process.argv.slice(2).map(Number);
const INPUT = 5.0, OUTPUT = 25.0, CACHE_READ = 0.5; // dollars per million tokens
const cached = ti - un;
const noCaching = (ti / 1e6) * INPUT + (out / 1e6) * OUTPUT;
const withCaching = (un / 1e6) * INPUT + (cached / 1e6) * CACHE_READ + (out / 1e6) * OUTPUT;
const cents = (d) => "$" + d.toFixed(2);
console.log(JSON.stringify({
  total_input: ti, uncached_input: un, cached_input: cached, total_output: out,
  no_caching: cents(noCaching), with_caching: cents(withCaching),
  no_caching_raw: +noCaching.toFixed(4), with_caching_raw: +withCaching.toFixed(4),
}));
