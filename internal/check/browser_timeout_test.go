package check

import "time"

// browserTestTimeout bounds a browser-backed test. It guards against hangs, not
// slowness: cold Chrome starts on shared CI runners under -race can take 20 s.
const browserTestTimeout = 60 * time.Second
