// Package dynhist implements dynamic histogram collector.
package dynhist

import (
	"fmt"
	"math"
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
func (c *Collector) Add(v float64) {
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

		c.Min = v
		c.Max = v

		return
	}

	if v < c.Min {
		c.Buckets = append([]Bucket{{Count: 1, Min: v, Max: v}}, c.Buckets...)
		c.Min = v

		return
	}

	if v > c.Max {
		c.Buckets = append(c.Buckets, Bucket{Count: 1, Min: v, Max: v})
		c.Max = v

		return
	}

	//  [1 3] [4 4] 5 [7 9]
	for i, b := range c.Buckets {
		if v >= b.Min {
			if v <= b.Max {
				c.Buckets[i].Count++
				return
			}
		} else {
			// Insert new bucket.
			c.Buckets = append(c.Buckets, Bucket{})
			copy(c.Buckets[i+1:], c.Buckets[i:])
			c.Buckets[i] = Bucket{Count: 1, Min: v, Max: v}
			return
		}
	}
}

// String renders buckets value.
func (c *Collector) String() string {
	c.Lock()
	defer c.Unlock()

	nLen := printfLen("%.2f", c.Min)
	if printfLen("%.2f", c.Max) > nLen {
		nLen = printfLen("%.2f", c.Max)
	}

	cLen := printfLen("%d", c.Count)
	res := fmt.Sprintf("[%"+nLen+"s %"+nLen+"s] %"+cLen+"s total%% (%d events)\n", "min", "max", "cnt", c.Count)

	for _, b := range c.Buckets {
		percent := float64(100*b.Count) / float64(c.Count)

		dots := ""
		for i := 0; i < int(percent); i++ {
			dots += "."
		}

		if len(dots) > 0 {
			dots = " " + dots
		}

		res += fmt.Sprintf("[%"+nLen+".2f %"+nLen+".2f] %"+cLen+"d %5.2f%%%s\n", b.Min, b.Max, b.Count, percent, dots)
	}

	return res
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
