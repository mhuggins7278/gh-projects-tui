# Board Responsiveness Measurement

## Reference setup

- Date: 2026-09-24
- OS: Linux x86_64
- CPU: Intel Core Ultra 9 185H (22 online logical CPUs)
- Go: go1.27.0-X:nodwarf5
- Display: 2880×1800 at 1.6× scale
- Terminal pane: 123×63 columns/rows

## Fixture and method

Run:

```sh
go test ./internal/ui -run 'TestLargeBoard' -count=3 -v
go test ./internal/ui -run '^$' -bench '^BenchmarkLargeBoardNavigation$' -benchtime=20x -count=2
```

The sanitized fixture contains 1,000 synthetic issue cards in ten 100-card
pages. The first page has a simulated 3 ms read delay; the next page and an
80 ms lazy detail request are held behind test gates while 102
key-input/model-update/render cycles (navigation and alternating matching search
terms) are measured. After all pages load, 67 additional cycles exercise the
last card in the lane and local search, followed by 30 cycles with a saved
title sort. Each interaction measurement includes `Model.Update` and generating
`Model.View().Content`; the first-page timing also includes the rendered view.
It does not include terminal emulator painting or a live GitHub network
request. The test checks that selection identity survives progressive page
completion.

## Results

Three runs produced:

| Metric | Results |
| --- | ---: |
| First-page read + rendered view | 8.98–12.33 ms |
| During page/detail reads: input + view-generation p95 | 6.70–7.30 ms |
| Fully loaded, position-sorted: input + view-generation p95 | 7.43–8.45 ms |
| Fully loaded, title-sorted: input + view-generation p95 | 10.24–10.84 ms |
| Board page requests | 10 |
| Lazy detail requests | 1 |

The full-board benchmark exposed a slow path: `boardWindowForLane` rendered all
1,000 cards to calculate their heights even though only a few fit in the
viewport. On this machine, the position-sorted 1,000-card navigation/render
benchmark was about **68 ms/op and 313k allocations/op** before the change.
Measuring only cards adjacent to the visible viewport reduced it to **5.9–6.1
ms/op and about 8.5k allocations/op**. The 100-card case also dropped from
roughly 12 ms/op to 4.8–5.5 ms/op. These are repeatable local model/render
baselines; the measured p95 is below the 100 ms target on this fixture and
reference machine, not a claim about live API latency or terminal paint time.
