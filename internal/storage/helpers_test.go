package storage

import "time"

// farFuture is an open upper bound for range queries in tests.
var farFuture = time.Now().AddDate(100, 0, 0)
