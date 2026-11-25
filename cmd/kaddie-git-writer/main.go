// Command kaddie-git-writer is a Kaddie component. It is a placeholder until its phase of
// the implementation plan lands; for now it only reports its version.
package main

import (
	"fmt"

	"github.com/morozj01/kaddie/pkg/version"
)

func main() {
	fmt.Println(version.String("kaddie-git-writer"))
}
