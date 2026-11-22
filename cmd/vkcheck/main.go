package main

import (
	"fmt"
	"os"

	"github.com/R19988088/GameRoom/internal/core"
)

func main() {
	if err := core.VulkanSelfTest(); err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	fmt.Println("VULKAN OK")
}
