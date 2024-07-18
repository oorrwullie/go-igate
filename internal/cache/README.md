## Cache
This implementation provides a scalable solution using a combination of an LRU cache and a Bloom Filter to efficiently manage APRS packet storage while reducing duplicate transmissions.

## Usage Example
```golang
func main() {
	cache := NewCache(1000, 10000, "cache.json") // Capacity of 1000 for LRU, Bloom filter size of 10,000 items

	// Example APRS entries
	entries := []string{
		"APRS: N7RIX-7>T0QPSP,WIDE1-1,WIDE2-1:`AYl\"<[/`\"C0}_3",
		"APRS: N7RIX-7>T0QPSP,WIDE1-1,WIDE2-2:`AYl\"<[/`\"C0}_4",
		"APRS: N7RIX-7>T0QPSP,WIDE1-1,WIDE2-3:`AYl\"<[/`\"C0}_5",
		"APRS: N7RIX-7>T0QPSP,WIDE1-1,WIDE2-1:`AYl\"<[/`\"C0}_3", // Duplicate
	}

	for _, entry := range entries {
		isDuplicate := cache.Set(entry, entry)

        if isDuplicate {
            fmt.Println("Duplicate entry")
        }
	}

	// Check cache contents
	for _, entry := range entries {
		value, exists := cache.Get(entry)
		if exists {
			fmt.Printf("Entry found in cache: %v\n", value)
		} else {
			fmt.Printf("Entry not found in cache: %v\n", entry)
		}
	}
}
```
