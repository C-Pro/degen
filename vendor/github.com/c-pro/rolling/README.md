# Rolling

This go package provides rolling window aggregation container that calculates basic statistical metrics for a float64 time series data.

Please note that rolling window implementation provided by this package is not thread-safe. You need to provide your own synchronization if you want to use it in concurrent environment.

The idea is to calculate metrics on the fly without O(n) aggregation every time a new data point is added to the time series.

We still have to keep all values in memory for min/max calculation with eviction, but we don't have to iterate over all of them every time we want to calculate a metric.

Please note that reported metrics are calculated at the time last value was added.
If you want to force-evict old values, call `Evict` before accessing metric methods.

Currently supported metrics are:

* Sum - sum of all values in the window.
* Count - total number of values in the window.
* Min - minimal value.
* Max - maximal value.
* Avg - average value.
* Mid - middle value (average of first and last values).
* First - first value in the window.
* Last - last value in the window.


## Example

```go
	w := NewWindow(6, time.Second)
	w.Add(1)
	w.Add(2)
	time.Sleep(time.Second)
	w.Add(3)
	w.Add(4)
	w.Add(5)
	w.Add(6)
	w.Add(7)
	fmt.Println(w.Min(), w.Avg(), w.Max())
	// Output: 3 5 7
```
