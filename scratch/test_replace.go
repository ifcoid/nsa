//go:build ignore

package main
import (
	"fmt"
	"strings"
)
func main() {
	s := "in\ufb00us"
	res := strings.ReplaceAll(s, "\ufb00", "ff")
	fmt.Println(res)
}