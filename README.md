# Dynamic histogram collector for Go

[![Build Status](https://travis-ci.org/vearutop/dynhist-go.svg?branch=master)](https://travis-ci.org/vearutop/dynhist-go)
[![Coverage Status](https://codecov.io/gh/vearutop/dynhist-go/branch/master/graph/badge.svg)](https://codecov.io/gh/vearutop/dynhist-go)
[![GoDevDoc](https://img.shields.io/badge/dev-doc-00ADD8?logo=go)](https://pkg.go.dev/github.com/vearutop/dynhist-go)
![Code lines](https://sloc.xyz/github/vearutop/dynhist-go/?category=code)
![Comments](https://sloc.xyz/github/vearutop/dynhist-go/?category=comments)

This library implements streaming counter with dynamic bucketing by value size.

## Weighted input and output buckets

Use `AddN(value, count)` for preaggregated observations, such as SQL results
containing a value and its frequency. Collection is independent of `count` unless
`RawValues` is enabled, in which case all `count` observations are retained.
Zero counts are ignored; negative counts and integer count overflow panic without
changing the collector. `Add(value)` is equivalent to `AddN(value, 1)`.

```go
c := dynhist.Collector{BucketsLimit: 200}
c.AddN(12, 990)
c.AddN(36, 9)
c.AddN(100, 1)

p99 := c.Percentile(99) // Query the finer collection buckets first.
c.Squash(20)           // Then reduce to at most 20 output buckets.
fmt.Println(p99, c.Buckets)
```

`Squash(limit)` merges adjacent buckets using the configured `WeightFunc`, or
`AvgWidth` by default. It preserves counts, sums, observed bounds and raw values,
and updates `BucketsLimit` for subsequent additions. It never splits existing
buckets, and nonpositive limits panic without changing the collector.

Both methods lock the collector. A weighted batch is merged as a whole; custom
weight functions based on counts or sums may choose different boundaries than
repeated individual `Add` calls. Weighted sums use `float64(count) * value`, so
floating-point rounding may also differ from repeated addition.

## Snapshots and percentiles

`Snapshot()` copies the totals (`Min`, `Max`, `Count`, `Sum`), bucket limit, buckets
and retained raw values under one lock. Its slices do not share storage with the
collector. Use it for consistent reads while other goroutines collect or squash:

```go
snapshot := c.Snapshot()
fmt.Println(snapshot.Count, snapshot.Buckets)
```

`Percentiles(50, 90, 95, 99)` returns estimates in request order under one lock.
Single queries use `Percentile(percent)`. Both select the upper boundary of the
bucket containing the nearest-rank observation (`ceil(percent * Count / 100)`),
using weighted counts. There is no interpolation inside buckets.

P0 returns the lower boundary of the first nonempty bucket; P100 returns the
upper boundary of the last nonempty bucket. Empty distributions and invalid
percentages (outside `[0, 100]`, NaN or infinity) return NaN. A batch with no
arguments returns nil; an invalid percentage affects only its own result.

`PercentileSum(percent)` uses the same rank and returns the sum of all buckets
through the selected bucket, including that bucket in full. P0 returns zero for
nonempty distributions; empty distributions and invalid percentages return NaN.

Percentile behavior changes: fractional ranks now round up instead of down,
empty distributions return NaN instead of zero, and empty buckets are skipped.
Query percentiles before `Squash` when you want the finer collection precision.
Direct access to a collector's exported fields still requires synchronization
when it is shared between goroutines.

## Usage

```go
c := dynhist.Collector{
    BucketsLimit: 10,
}
src := rand.NewSource(1)
r := rand.New(src)

for i := 0; i < 10000; i++ {
    c.Add(r.Float64())
}

fmt.Println(c.String())
// Output:
// [ min  max]   cnt total% (10000 events)
// [0.00 0.11]  1099 10.99% ..........
// [0.11 0.22]  1093 10.93% ..........
// [0.22 0.33]  1127 11.27% ...........
// [0.33 0.44]  1121 11.21% ...........
// [0.44 0.54]   999  9.99% .........
// [0.54 0.63]   964  9.64% .........
// [0.63 0.73]   953  9.53% .........
// [0.73 0.81]   841  8.41% ........
// [0.81 0.90]   797  7.97% .......
// [0.90 1.00]  1006 10.06% ..........

```
