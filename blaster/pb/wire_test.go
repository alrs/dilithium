package pb

import (
	"bytes"
	"fmt"
	"testing"
)

func ReadWriteTest(t *testing.T) {
	buffer := new(bytes.Buffer)
	err := WriteMessage(NewHello(9, "OH, WOW"), buffer)
	if err != nil {
		panic(err)
	}
	wm, err := ReadMessage(buffer)
	if err != nil {
		panic(err)
	}
	fmt.Printf(wm.String())
}