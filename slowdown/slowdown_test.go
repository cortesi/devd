package slowdown

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/juju/ratelimit"
)

func TestWriter(t *testing.T) {
	sizes := []int64{0, 1, capacity, blockSize, 4096, 99, 100}
	for _, size := range sizes {
		b := &bytes.Buffer{}
		sw := slowWriter{b, ratelimit.NewBucketWithRate(1024*1024, capacity)}

		data := make([]byte, size)
		_, err := rand.Read(data)
		if err != nil {
			t.Errorf("Could not read random data")
		}
		len, err := sw.Write(data)
		if err != nil {
			t.Errorf("Write error: %s", err)
		}
		if int64(len) != size {
			t.Errorf("Expected to write %d bytes, wrote %d", size, len)
		}

		if bytes.Equal(data, b.Bytes()) != true {
			t.Fail()
		}

	}
}

func TestReader(t *testing.T) {
	sizes := []int64{0, 1, capacity, blockSize, 4096, 99, 100}
	for _, size := range sizes {
		src := make([]byte, size)
		_, err := rand.Read(src)
		if err != nil {
			t.Errorf("Could not read random data")
		}
		sr := slowReader{
			bytes.NewBuffer(src),
			ratelimit.NewBucketWithRate(1024*1024, capacity),
		}

		dst := make([]byte, size)
		len, err := sr.Read(dst)
		if err != nil {
			t.Errorf("Read error: %s", err)
		}
		if int64(len) != size {
			t.Errorf("Expected %d bytes, got %d", size, len)
		}

		if bytes.Equal(dst, src) != true {
			t.Fail()
		}
	}
}
