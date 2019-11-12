# Dynamic histogram collector for Go

[![Build Status](https://travis-ci.org/vearutop/dynhist-go.svg?branch=master)](https://travis-ci.org/vearutop/dynhist-go)
[![Coverage Status](https://codecov.io/gh/vearutop/dynhist-go/branch/master/graph/badge.svg)](https://codecov.io/gh/vearutop/dynhist-go)
[![GoDoc](https://godoc.org/github.com/vearutop/dynhist-go?status.svg)](https://godoc.org/github.com/vearutop/dynhist-go)
![Code lines](https://sloc.xyz/github/vearutop/dynhist-go/?category=code)
![Comments](https://sloc.xyz/github/vearutop/dynhist-go/?category=comments)

This library implements streaming counter with dynamic bucketing by value size.

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
