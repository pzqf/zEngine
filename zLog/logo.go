package zLog

import (
	"fmt"
	"runtime"
	"time"
)

const zEngineLogo = `
╔════════════════════════════════════════════════════════════════╗
║                                                                ║
║   ███████╗███████╗███╗   ██╗ ██████╗ ██╗███╗   ██╗███████╗     ║
║   ╚══███╔╝██╔════╝████╗  ██║██╔════╝ ██║████╗  ██║██╔════╝     ║
║     ███╔╝ █████╗  ██╔██╗ ██║██║  ███╗██║██╔██╗ ██║█████╗       ║
║    ███╔╝  ██╔══╝  ██║╚██╗██║██║   ██║██║██║╚██╗██║██╔══╝       ║
║   ███████╗███████╗██║ ╚████║╚██████╔╝██║██║ ╚████║███████╗     ║
║   ╚══════╝╚══════╝╚═╝  ╚═══╝ ╚═════╝ ╚═╝╚═╝  ╚═══╝╚══════╝     ║
║                                                                ║
║                    zEngine - Go Game Engine                    ║
║           A Lightweight Distributed Game Server Framework      ║
║                                                                ║
╚════════════════════════════════════════════════════════════════╝
`

func PrintZEngineLogo(appName string, version string) {
	fmt.Print(zEngineLogo)
	fmt.Println()
	if appName != "" {
		fmt.Printf("  Application: %s\n", appName)
	}
	if version != "" {
		fmt.Printf("  Version: %s\n", version)
	}
	fmt.Printf("  zEngine Version: %s\n", "1.0.0")
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  CPUs: %d\n", runtime.NumCPU())
	fmt.Printf("  Start Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println()
	fmt.Println("  ═══════════════════════════════════════════════════════════")
	fmt.Println()
}

func PrintLogo(appName string, version string) {
	PrintZEngineLogo(appName, version)
}
