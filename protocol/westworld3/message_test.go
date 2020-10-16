package westworld3

import (
	"encoding/hex"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func init() {
	for i := 0; i < 16*1024; i++ {
		wireMessageBenchmarkData[i] = uint8(i)
	}
	wireMessageBenchmarkPool = newPool("wireMessageBenchmark", dataStart+(16*1024), nil)
}

var wireMessageBenchmarkData [16 * 1024]byte
var wireMessageBenchmarkPool *pool

func TestHello(t *testing.T) {
	p := newPool("test", 1024, nil)
	wm, err := newHello(11, hello{protocolVersion, 6}, nil, p)
	assert.NoError(t, err)
	fmt.Println(hex.Dump(wm.buffer.data[:wm.buffer.uz]))
	assert.Equal(t, uint32(dataStart+5), wm.buffer.uz)

	wmOut, err := decodeHeader(wm.buffer)
	assert.NoError(t, err)
	h, a, err := wmOut.asHello()
	assert.NoError(t, err)
	assert.Equal(t, int32(11), wmOut.seq)
	assert.Equal(t, HELLO, wmOut.messageType())
	assert.Equal(t, protocolVersion, h.version)
	assert.Equal(t, uint8(6), h.profile)
	assert.Equal(t, 0, len(a))
}

func TestHelloResponse(t *testing.T) {
	p := newPool("test", 1024, nil)
	wm, err := newHello(12, hello{protocolVersion, 6}, &ack{11, 11}, p)
	assert.NoError(t, err)
	fmt.Println(hex.Dump(wm.buffer.data[:wm.buffer.uz]))
	assert.Equal(t, uint32(dataStart+4+5), wm.buffer.uz)
	assert.True(t, wm.hasFlag(INLINE_ACK))

	wmOut, err := decodeHeader(wm.buffer)
	assert.NoError(t, err)
	h, a, err := wmOut.asHello()
	assert.NoError(t, err)
	assert.Equal(t, int32(12), wmOut.seq)
	assert.Equal(t, HELLO, wmOut.messageType())
	assert.Equal(t, protocolVersion, h.version)
	assert.Equal(t, uint8(6), h.profile)
	assert.Equal(t, 1, len(a))
	assert.Equal(t, int32(11), a[0].start)
	assert.Equal(t, int32(11), a[0].end)
}

func TestWireMessageInsertData(t *testing.T) {
	p := newPool("test", 1024, nil)
	wm := &wireMessage{seq: 0, mt: DATA, buffer: p.get()}
	copy(wm.buffer.data[dataStart:], []byte{0x01, 0x02, 0x03, 0x04})
	wmOut, err := wm.encodeHeader(4)
	assert.NoError(t, err)
	assert.Equal(t, wm.buffer.uz, uint32(dataStart+4))
	assert.Equal(t, wm, wmOut)

	err = wm.insertData([]byte{0x0a, 0x0b, 0x0c, 0x0d})
	assert.NoError(t, err)
	assert.Equal(t, wm.buffer.uz, uint32(dataStart+8))
	assert.ElementsMatch(t, []byte{0x0a, 0x0b, 0x0c, 0x0d, 0x01, 0x02, 0x03, 0x04}, wm.buffer.data[dataStart:wm.buffer.uz])
}

func benchmarkWireMessageInsertData(dataSz, insertSz int, b *testing.B) {
	for i := 0; i < b.N; i++ {
		wm := &wireMessage{seq: 0, mt: DATA, buffer: wireMessageBenchmarkPool.get()}
		copy(wm.buffer.data[dataStart:], wireMessageBenchmarkData[:dataSz])
		if _, err := wm.encodeHeader(uint16(dataSz)); err != nil {
			panic(err)
		}
		if err := wm.insertData(wireMessageBenchmarkData[:insertSz]); err != nil {
			panic(err)
		}
		wm.buffer.unref()
	}
}
func BenchmarkWireMessageInsertData8(b *testing.B)    { benchmarkWireMessageInsertData(8, 8, b) }
func BenchmarkWireMessageInsertData256(b *testing.B)  { benchmarkWireMessageInsertData(256, 8, b) }
func BenchmarkWireMessageInsertData1024(b *testing.B) { benchmarkWireMessageInsertData(1024, 8, b) }
func BenchmarkWireMessageinsertData4096(b *testing.B) { benchmarkWireMessageInsertData(4096, 8, b) }

func TestWireMessageAppendData(t *testing.T) {
	p := newPool("test", 1024, nil)
	wm := &wireMessage{seq: 0, mt: DATA, buffer: p.get()}
	copy(wm.buffer.data[dataStart:], []byte{0x01, 0x02, 0x03, 0x04})
	wmOut, err := wm.encodeHeader(4)
	assert.NoError(t, err)
	assert.Equal(t, wm.buffer.uz, uint32(dataStart+4))
	assert.Equal(t, wm, wmOut)

	err = wm.appendData([]byte{0x0a, 0x0b, 0x0c, 0x0d})
	assert.NoError(t, err)
	assert.Equal(t, wm.buffer.uz, uint32(dataStart+8))
	assert.ElementsMatch(t, []byte{0x01, 0x02, 0x03, 0x04, 0x0a, 0x0b, 0x0c, 0x0d}, wm.buffer.data[dataStart:wm.buffer.uz])
}

func benchmarkWireMessageAppendData(dataSz, insertSz int, b *testing.B) {
	for i := 0; i < b.N; i++ {
		wm := &wireMessage{seq: 0, mt: DATA, buffer: wireMessageBenchmarkPool.get()}
		copy(wm.buffer.data[dataStart:], wireMessageBenchmarkData[:dataSz])
		if _, err := wm.encodeHeader(uint16(dataSz)); err != nil {
			panic(err)
		}
		if err := wm.appendData(wireMessageBenchmarkData[:insertSz]); err != nil {
			panic(err)
		}
		wm.buffer.unref()
	}
}
func BenchmarkWireMessageAppendData8(b *testing.B)    { benchmarkWireMessageAppendData(8, 8, b) }
func BenchmarkWireMessageAppendData256(b *testing.B)  { benchmarkWireMessageAppendData(256, 8, b) }
func BenchmarkWireMessageAppendData1024(b *testing.B) { benchmarkWireMessageAppendData(1024, 8, b) }
func BenchmarkWireMessageAppendData4096(b *testing.B) { benchmarkWireMessageAppendData(4096, 8, b) }