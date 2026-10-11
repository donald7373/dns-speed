//go:build !windows

package main

import "fmt"

// Program okienkowy dziala tylko w Windows. Ten plik pozwala budowac i testowac
// reszte kodu (model, uklad, rysowanie) na innych systemach.
func main() {
	fmt.Println("DNS Speed v2 dziala tylko w Windows.")
}
