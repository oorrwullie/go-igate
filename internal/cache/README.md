## Cache
An in-memory LRU cache with optional TTL persistence used to suppress duplicate APRS packets between decoders.

## Usage Example
```golang
func main() {
    cache := NewCache(1000, "cache.json", 30*time.Minute)

    entries := []string{
        "N7RIX-7>T0QPSP,WIDE1-1,WIDE2-1:`AYl\"<[/*C0}_3",
        "N7RIX-7>T0QPSP,WIDE1-1,WIDE2-2:`AYl\"<[/*C0}_4",
        "N7RIX-7>T0QPSP,WIDE1-1,WIDE2-3:`AYl\"<[/*C0}_5",
        "N7RIX-7>T0QPSP,WIDE1-1,WIDE2-1:`AYl\"<[/*C0}_3",
    }

    for _, entry := range entries {
        if cache.Set(entry, time.Now()) {
            fmt.Println("Duplicate entry")
        }
    }

    for _, entry := range entries {
        if _, exists := cache.Get(entry); exists {
            fmt.Println("Entry found:", entry)
        }
    }
}
```
