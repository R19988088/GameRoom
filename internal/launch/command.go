package launch

import (
	"fmt"
	"net"
)

// CommandPort is where RetroArch's Network Control Interface listens. It is
// written into the config each game starts with, so the port is one number the
// config and the sender agree on.
const CommandPort = 55355

// SendCommand hands one Network Control Interface command to a running
// RetroArch. It is a single UDP datagram — there is no reply to wait for, and
// a lossy send is not fatal: the caller only needs to know whether anything
// was listening at all.
func SendCommand(port int, command string) error {
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(command)); err != nil {
		return fmt.Errorf("发送 %q 失败：%w", command, err)
	}
	return nil
}

// SetShaderLive points a running RetroArch at a preset file.
func SetShaderLive(port int, path string) error {
	return SendCommand(port, "SET_SHADER "+path)
}
