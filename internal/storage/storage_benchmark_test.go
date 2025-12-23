package storage

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

var payload = []byte("TOLEREX_STORAGE_TEST\n")

// ---------------------------------------------------
// UNBUFFERED WRITE
// ---------------------------------------------------

func BenchmarkStorage_UnbufferedWrite(b *testing.B) {
	b.ReportAllocs()

	baseDir := b.TempDir()
	file := filepath.Join(baseDir, "unbuffered.log")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := f.Write(payload); err != nil {
			f.Close()
			b.Fatal(err)
		}
		f.Close()
	}
}

// ---------------------------------------------------
// BUFFERED WRITE (flush periodically)
// ---------------------------------------------------

func BenchmarkStorage_BufferedWrite(b *testing.B) {
	b.ReportAllocs()

	baseDir := b.TempDir()
	file := filepath.Join(baseDir, "buffered.log")

	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()

	writer := bufio.NewWriterSize(f, 4*1024)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := writer.Write(payload); err != nil {
			b.Fatal(err)
		}

		// flush every 100 writes (REALISTIC)
		if i%100 == 0 {
			if err := writer.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	}

	if err := writer.Flush(); err != nil {
		b.Fatal(err)
	}
}
