// Package dynhist implements dynamic histogram collector.
package dynhist

import (
	"fmt"
	"math"
	"runtime/metrics"
	"strconv"
	"sync"
)

// DefaultBucketsLimit is a default maximum number of buckets.
const DefaultBucketsLimit = 20

// Collector groups and counts values by size using buckets.
type Collector struct {
	sync.Mutex

	// BucketsLimit limits total number of buckets used.
	BucketsLimit int

	// Bucket keeps total count.
	Bucket

	// Buckets is a list of available buckets.
	Buckets []Bucket

	// PrintSum enables printing of a summary value in a bucket.
	PrintSum bool

	// RawValues stores incoming events, disabled by default. Use non-nil value to enable.
	RawValues []float64

	// WeightFunc calculates weight of adjacent buckets with total available. Pair with minimal weight is merged.
	// AvgWidth is used by default.
	// See also LatencyWidth, ExpWidth.
	WeightFunc func(b1, b2, bTot Bucket) float64
}

// Bucket keeps count of values in boundaries.
type Bucket struct {
	Min   float64
	Max   float64
	Count int
	Sum   float64
}

// ExpWidth creates a weight function with exponential bucket width growing.
//
// For exponentially distributed data, values of 1.2 for sumWidthPow and 1 for spacingPow should be a good fit.
// Increase sumWidthPow to widen buckets for lower values.
// Increase spacingPow to widen buckets for higher values.
func ExpWidth(sumWidthPow, spacingPow float64) func(b1, b2, bTot Bucket) float64 {
	return func(b1, b2, bTot Bucket) float64 {
		return math.Pow(b2.Max-b1.Min, sumWidthPow) / math.Pow(b2.Max-bTot.Min, spacingPow)
	}
}

// LatencyWidth is a weight function suitable for collecting latency information.
//
// It makes wider buckets for higher values, narrow buckets for lower values.
func LatencyWidth(b1, b2, bTot Bucket) float64 {
	return (b2.Max - b1.Min) / (b2.Max - bTot.Min)
}

// AvgWidth is a weight function to maintain equal width of all buckets.
//
// Should fit for normally distributed data.
func AvgWidth(b1, b2, bTot Bucket) float64 {
	return b2.Max - b1.Min
}

// Add collects value.
func (c *Collector) Add(v float64) { //nolint:funlen,cyclop
	c.Lock()
	defer func() {
		if len(c.Buckets) > c.BucketsLimit {
			minWeight := 0.0
			mergePoint := 0

			for i := 1; i < len(c.Buckets); i++ {
				if mergePoint == 0 {
					mergePoint = i
					minWeight = c.WeightFunc(c.Buckets[i-1], c.Buckets[i], c.Bucket)

					continue
				}

				weight := c.WeightFunc(c.Buckets[i-1], c.Buckets[i], c.Bucket)
				if weight < minWeight {
					minWeight = weight
					mergePoint = i
				}
			}

			b1 := c.Buckets[mergePoint-1]
			b2 := c.Buckets[mergePoint]
			merged := Bucket{
				Count: b1.Count + b2.Count,
				Sum:   b1.Sum + b2.Sum,
				Min:   b1.Min,
				Max:   b2.Max,
			}

			c.Buckets = append(c.Buckets[:mergePoint-1], c.Buckets[mergePoint:]...)

			c.Buckets[mergePoint-1] = merged
		}
		c.Unlock()
	}()

	if c.RawValues != nil {
		c.RawValues = append(c.RawValues, v)
	}
	c.Count++
	c.Sum += v

	if len(c.Buckets) == 0 {
		if c.BucketsLimit == 0 {
			c.BucketsLimit = DefaultBucketsLimit
		}

		if c.WeightFunc == nil {
			c.WeightFunc = AvgWidth
		}

		c.Buckets = make([]Bucket, 1, c.BucketsLimit)
		c.Buckets[0].Min = v
		c.Buckets[0].Max = v
		c.Buckets[0].Count = 1
		c.Buckets[0].Sum = v

		c.Min = v
		c.Max = v

		return
	}

	if v < c.Min {
		c.Buckets = append([]Bucket{{Count: 1, Min: v, Max: v, Sum: v}}, c.Buckets...)
		c.Min = v

		return
	}

	if v > c.Max {
		c.Buckets = append(c.Buckets, Bucket{Count: 1, Min: v, Max: v, Sum: v})
		c.Max = v

		return
	}

	//  [1 3] [4 4] 5 [7 9]
	for i, b := range c.Buckets {
		if v >= b.Min {
			if v <= b.Max {
				c.Buckets[i].Count++
				c.Buckets[i].Sum += v

				return
			}
		} else {
			// Insert new bucket.
			c.Buckets = append(c.Buckets, Bucket{})
			copy(c.Buckets[i+1:], c.Buckets[i:])
			c.Buckets[i] = Bucket{Count: 1, Min: v, Max: v, Sum: v}

			return
		}
	}
}

// String renders buckets value.
func (c *Collector) String() string {
	c.Lock()
	defer c.Unlock()

	if len(c.Buckets) == 0 {
		return ""
	}

	nLen := printfLen("%.2f", c.Min)
	if printfLen("%.2f", c.Max) > nLen {
		nLen = printfLen("%.2f", c.Max)
	}
	// if c.Max is +Inf, the second-largest element can be the longest.
	if maxLen := printfLen("%.2f", c.Buckets[len(c.Buckets)-1].Min); maxLen > nLen {
		nLen = maxLen
	}

	cLen := printfLen("%d", c.Count)
	sLen := ""

	var res string

	if c.PrintSum {
		sLen = printfLen("%.2f", c.Sum)
		res = fmt.Sprintf("[%"+nLen+"s %"+nLen+"s] %"+cLen+"s total%% %"+sLen+"s (%d events)\n", "min", "max", "cnt", "sum", c.Count)
	} else {
		res = fmt.Sprintf("[%"+nLen+"s %"+nLen+"s] %"+cLen+"s total%% (%d events)\n", "min", "max", "cnt", c.Count)
	}

	for _, b := range c.Buckets {
		percent := float64(100*b.Count) / float64(c.Count)

		dots := ""
		for i := 0; i < int(percent); i++ {
			dots += "."
		}

		if len(dots) > 0 {
			dots = " " + dots
		}

		if c.PrintSum {
			res += fmt.Sprintf("[%"+nLen+".2f %"+nLen+".2f] %"+cLen+"d %5.2f%% %"+sLen+".2f%s\n", b.Min, b.Max, b.Count, percent, b.Sum, dots)
		} else {
			res += fmt.Sprintf("[%"+nLen+".2f %"+nLen+".2f] %"+cLen+"d %5.2f%%%s\n", b.Min, b.Max, b.Count, percent, dots)
		}
	}

	return res
}

// LoadFromRuntimeMetrics replaces existing buckets with data from metrics.Float64Histogram.
func (c *Collector) LoadFromRuntimeMetrics(h *metrics.Float64Histogram) {
	c.Lock()
	defer c.Unlock()

	c.Buckets = make([]Bucket, len(h.Buckets)-1)
	c.BucketsLimit = len(h.Buckets)
	c.Bucket = Bucket{
		Min: h.Buckets[0],
		Max: h.Buckets[0],
	}

	for i, b := range h.Buckets[1:] {
		bb := Bucket{
			Min:   c.Bucket.Max,
			Max:   b,
			Count: int(h.Counts[i]),
		}

		if bb.Count != 0 && !math.IsInf(b, 0) {
			bb.Sum = float64(bb.Count) * b
			c.Bucket.Sum += bb.Sum
		}

		c.Bucket.Count += bb.Count
		c.Bucket.Max = b

		c.Buckets[i] = bb
	}
}

func printfLen(format string, val interface{}) string {
	s := fmt.Sprintf(format, val)

	return strconv.Itoa(len(s))
}

// Percentile returns maximum boundary for a fraction of values.
func (c *Collector) Percentile(percent float64) float64 {
	c.Lock()
	defer c.Unlock()
	targetCount := int(percent * float64(c.Count) / 100)

	count := 0
	for _, b := range c.Buckets {
		count += b.Count
		if count >= targetCount {
			return b.Max
		}
	}

	return c.Max
}
