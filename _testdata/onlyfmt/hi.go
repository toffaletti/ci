package hi

import (
	"fmt"
	"log"
	_ "net/url"
	_ "encoding/json"
	"os"
)

func Hello() bool {
	os.Stderr.Write([]byte(fmt.Sprintf("bad code\n")))
	log.Printf("yay")
	return true
}
