package main

// func main() {
// 	cmd := exec.Command("bash", "-i")
// 	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

// 	ptmx, err := pty.Start(cmd)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer func() {
// 		_ = ptmx.Close()
// 		_ = cmd.Process.Kill()
// 	}()

// 	fmt.Fprintln(os.Stderr, "[*] Shell session started.")
// 	go io.Copy(ptmx, os.Stdin)
// 	io.Copy(os.Stdout, ptmx)
// }
