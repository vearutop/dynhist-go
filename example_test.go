package dynhist_test

import (
	"fmt"
	"math/rand"

	"github.com/vearutop/dynhist-go"
)

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
	// [  min   max]    cnt total% (100000 events)
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
