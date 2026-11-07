package dynhist_test

import (
	"fmt"
	"math/rand"

	"github.com/vearutop/dynhist-go"
)

func ExampleCollector_AddN() {
	c := dynhist.Collector{BucketsLimit: 200}
	c.AddN(12, 990)
	c.AddN(36, 9)
	c.AddN(100, 1)

	// Query percentiles before reducing the output to fewer buckets.
	fmt.Println("P99:", c.Percentile(99))
	c.Squash(1)
	fmt.Println(c.Buckets)
	// Output:
	// P99: 12
	// [{12 100 1000 12304}]
}

func ExampleCollector_Percentiles() {
	c := dynhist.Collector{}
	c.Add(10)
	c.Add(20)
	c.Add(30)
	fmt.Println(c.Percentiles(50, 90, 99))
	// Output: [20 30 30]
}

func ExampleCollector_Snapshot() {
	c := dynhist.Collector{}
	c.AddN(12, 3)
	snapshot := c.Snapshot()
	c.Add(36)
	fmt.Println(snapshot.Count, snapshot.Buckets)
	// Output: 3 [{12 12 3 36}]
}

func ExampleAvgWidth() {
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
	// [ min  max]   cnt total% (total count: 10000)
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
}

func ExampleExpWidth() {
	c := dynhist.Collector{
		BucketsLimit: 10,
		WeightFunc:   dynhist.ExpWidth(1.2, 0.9),
	}
	src := rand.NewSource(1)
	r := rand.New(src)

	for i := 0; i < 100000; i++ {
		c.Add(r.ExpFloat64())
	}

	fmt.Println(c.String())
	// Output:
	// [  min   max]    cnt total% (total count: 100000)
	// [ 0.00  0.07]   6577  6.58% ......
	// [ 0.07  0.22]  13380 13.38% .............
	// [ 0.22  0.45]  16002 16.00% ................
	// [ 0.45  1.11]  31072 31.07% ...............................
	// [ 1.11  1.77]  15975 15.97% ...............
	// [ 1.77  2.78]  10737 10.74% ..........
	// [ 2.78  4.37]   4993  4.99% ....
	// [ 4.37  6.50]   1121  1.12% .
	// [ 6.51  8.96]    134  0.13%
	// [ 9.03 10.80]      9  0.01%
}
