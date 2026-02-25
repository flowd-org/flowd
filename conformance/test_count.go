package main

import "fmt"

func main() {
	s := `{"header": "Authorization: Bearer abc123"}`
	fmt.Println("Length:", len(s))
	for i, c := range s {
		fmt.Printf("%d: %q\n", i, c)
	}
}
