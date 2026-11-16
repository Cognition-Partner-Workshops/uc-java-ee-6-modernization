package rest

import (
	"sort"
	"strings"
	"sync"
)

// address is the legacy Address bean. Field order is the order the legacy JSON provider writes.
type address struct {
	EntryName     string `json:"entryName"`
	ZipCode       string `json:"zipCode"`
	StreetAddress string `json:"streetAddress"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
}

// addressList is the legacy AddressList wrapper: {"addresses":[...]}.
type addressList struct {
	Addresses []address `json:"addresses"`
}

// addressBook is the legacy AddressBookDatabase: an in-memory map that is filled once and never
// persisted. The legacy resource has no write verbs, so nothing here touches PostgreSQL.
type addressBook struct {
	mutex   sync.RWMutex
	entries map[string]address
	order   []string
}

// newAddressBook stores the five entries of AddressBook's static initialiser, in its order.
func newAddressBook() *addressBook {
	book := &addressBook{entries: map[string]address{}}
	for _, entryName := range []string{"Entry5", "Entry4", "Entry3", "Entry2", "Entry1"} {
		book.store(address{
			EntryName:     entryName,
			ZipCode:       "12345",
			StreetAddress: "Address",
			City:          "City",
			State:         "State",
			Country:       "Country",
		})
	}
	return book
}

func (b *addressBook) store(entry address) {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if _, exists := b.entries[entry.EntryName]; !exists {
		b.order = append(b.order, entry.EntryName)
	}
	b.entries[entry.EntryName] = entry
}

func (b *addressBook) get(entryName string) (address, bool) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	entry, found := b.entries[entryName]
	return entry, found
}

// list returns the entries whose name starts with prefix, in a fixed order: the iteration order
// of the legacy java.util.HashMap (bucket index, then insertion order), so the list reads the
// same as on Liberty: Entry4, Entry5, Entry2, Entry3, Entry1.
func (b *addressBook) list(prefix string) []address {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	names := append([]string(nil), b.order...)
	buckets := javaHashMapCapacity(len(names))
	sort.SliceStable(names, func(i, j int) bool {
		return javaHashMapBucket(names[i], buckets) < javaHashMapBucket(names[j], buckets)
	})
	addresses := make([]address, 0, len(names))
	for _, entryName := range names {
		if strings.HasPrefix(entryName, prefix) {
			addresses = append(addresses, b.entries[entryName])
		}
	}
	return addresses
}

// javaHashMapCapacity is the table size of a default java.util.HashMap holding size entries
// (16 buckets, doubled whenever the size passes 0.75 of the table).
func javaHashMapCapacity(size int) uint32 {
	capacity := uint32(16)
	for uint32(size)*4 > capacity*3 {
		capacity *= 2
	}
	return capacity
}

// javaHashMapBucket is java.util.HashMap's bucket for a String key: String.hashCode() over the
// UTF-16 code units, the upper half folded in, masked by the table size.
func javaHashMapBucket(key string, capacity uint32) uint32 {
	var hash uint32
	for _, unit := range utf16Units(key) {
		hash = 31*hash + uint32(unit)
	}
	return (hash ^ hash>>16) & (capacity - 1)
}
