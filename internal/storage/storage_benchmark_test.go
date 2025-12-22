package storage

import (
	"bufio"
	"os"
	"testing"
)

var payload = []byte("TOLEREX_STORAGE_TEST\n")

// ---------------------------------------------------
// UNBUFFERED DISK WRITE (no fsync)
// ---------------------------------------------------

func BenchmarkStorage_UnbufferedWrite(b *testing.B) {
	b.ReportAllocs()

	f, err := os.Create("unbuffered_test.log")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		f.Close()
		os.Remove("unbuffered_test.log")
	}()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := f.Write(payload); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------
// BUFFERED DISK WRITE (no fsync)
// ---------------------------------------------------

func BenchmarkStorage_BufferedWrite(b *testing.B) {
	b.ReportAllocs()

	f, err := os.Create("buffered_test.log")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		f.Close()
		os.Remove("buffered_test.log")
	}()

	writer := bufio.NewWriterSize(f, 1*1024)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := writer.Write(payload); err != nil {
			b.Fatal(err)
		}
		if err := writer.Flush(); err != nil {
			b.Fatal(err)
		}
	}
}
