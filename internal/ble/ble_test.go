package ble

import (
	"bytes"
	"sync"
	"testing"
)

func TestNormalizeUUID(t *testing.T) {
	for in, want := range map[string]string{
		"FF09":                                 "0000ff09-0000-1000-8000-00805f9b34fb",
		"0xff09":                               "0000ff09-0000-1000-8000-00805f9b34fb",
		"1234ABCD":                             "1234abcd-0000-1000-8000-00805f9b34fb",
		"8C850001-0302-41C5-B46E-CF057C562025": "8c850001-0302-41c5-b46e-cf057c562025",
		"0000ff09-0000-1000-8000-00805f9b34fb": "0000ff09-0000-1000-8000-00805f9b34fb",
	} {
		if got := NormalizeUUID(in); got != want {
			t.Errorf("NormalizeUUID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortUUID(t *testing.T) {
	for in, want := range map[string]string{
		"0000ff09-0000-1000-8000-00805f9b34fb": "FF09",
		"ff09":                                 "FF09",
		"1234abcd-0000-1000-8000-00805f9b34fb": "1234ABCD",
		"8c850002-0302-41c5-b46e-cf057c562025": "8C850002-0302-41C5-B46E-CF057C562025",
	} {
		if got := shortUUID(in); got != want {
			t.Errorf("shortUUID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitManufacturerData(t *testing.T) {
	company, data, ok := splitManufacturerData([]byte{0x7d, 0x02, 0xaa, 0xbb})
	if !ok || company != 0x027d || !bytes.Equal(data, []byte{0xaa, 0xbb}) {
		t.Fatalf("got %#04x %x %v", company, data, ok)
	}
	if _, _, ok := splitManufacturerData([]byte{1}); ok {
		t.Fatal("one byte accepted")
	}
}

func TestFIFOOrderAndDrain(t *testing.T) {
	q := newFIFO[int]()
	var got []int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		q.run(func(v int) { got = append(got, v) })
	}()
	for i := range 10000 {
		q.push(i)
	}
	q.close()
	q.push(-1) // dropped
	wg.Wait()
	if len(got) != 10000 {
		t.Fatalf("delivered %d", len(got))
	}
	for i, v := range got {
		if v != i {
			t.Fatalf("got[%d] = %d", i, v)
		}
	}
}

func TestPropertyString(t *testing.T) {
	if s := (PropWriteWithoutResponse | PropNotify).String(); s != "write-without-response,notify" {
		t.Fatal(s)
	}
	if StatePoweredOn.String() != "poweredOn" || AuthDenied.String() != "denied" {
		t.Fatal("names")
	}
}
