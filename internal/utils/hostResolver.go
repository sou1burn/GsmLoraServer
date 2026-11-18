package utils

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

func ResolveEspHost(hostname, mac string) (string, error) {
	mac = strings.ToLower(mac)
	if ip, err := scanARPTable(mac); err == nil {
		return ip, nil
	}

	if ip, err := resolveMDNS(hostname); err == nil {
		return ip, nil
	}

	iface, ipNet, err := getNetworkInterface()
	if err != nil {
		return "", fmt.Errorf("failed to get network interface: %w", err)
	}
	printNetworkInfo(iface, ipNet)
	fmt.Printf("Ищем устройство с MAC: %s\n", mac)

	return activeNetworkScan(iface, ipNet, mac)
}

func resolveMDNS(hostname string) (string, error) {
	if !strings.HasSuffix(hostname, ".local") {
		hostname = hostname + ".local"
	}

	addrs, err := net.LookupHost(hostname)
	if err != nil {
		return "", err
	}

	if len(addrs) > 0 {
		return addrs[0], nil
	}

	return "", fmt.Errorf("mDNS resolution failed")
}

func getNetworkInterface() (*net.Interface, *net.IPNet, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, err
	}

	for _, iface := range interfaces {

		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			if ip.To4() != nil {
				return &iface, ipNet, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("no suitable network interface found")
}

func activeNetworkScan(iface *net.Interface, ipNet *net.IPNet, targetMAC string) (string, error) {

	_, err := getIPsFromNetwork(ipNet)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("arp-scan", "--interface", iface.Name, "--localnet")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("arp-scan failed: %w", err)
	}

	return parseArpScanOutput(string(output), targetMAC)
}

func getIPsFromNetwork(ipNet *net.IPNet) ([]string, error) {
	var ips []string

	ip := ipNet.IP.Mask(ipNet.Mask)
	for {
		ip = nextIP(ip)
		if !ipNet.Contains(ip) {
			break
		}
		ips = append(ips, ip.String())
	}

	return ips, nil
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)

	for j := len(next) - 1; j >= 0; j-- {
		next[j]++
		if next[j] > 0 {
			break
		}
	}

	return next
}

func parseArpScanOutput(output, targetMAC string) (string, error) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		if net.ParseIP(parts[0]) != nil {
			mac := parts[1]
			if mac == targetMAC {
				return parts[0], nil
			}
		}
	}

	return "", fmt.Errorf("device not found in network scan")
}

func scanARPTable(targetMAC string) (string, error) {

	file, err := os.Open("/proc/net/arp")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		ip := fields[0]
		mac := fields[3]

		if mac == targetMAC {
			return ip, nil
		}
	}

	return "", fmt.Errorf("MAC not found in ARP table")
}

func printNetworkInfo(iface *net.Interface, ipNet *net.IPNet) {
	fmt.Println("\n Network Information:")
	fmt.Printf("   Interface: %s\n", iface.Name)
	fmt.Printf("   Network: %s\n", ipNet.String())
	fmt.Printf("   MTU: %d\n", iface.MTU)

	if iface.HardwareAddr != nil {
		fmt.Printf("   MAC: %s\n", iface.HardwareAddr.String())
	}

	addrs, err := iface.Addrs()
	if err == nil {
		fmt.Println("   IP Addresses:")
		for i, addr := range addrs {
			fmt.Printf("     %d. %s\n", i+1, addr.String())
		}
	}

	flags := iface.Flags.String()
	fmt.Printf("   Flags: %s\n", flags)
}
