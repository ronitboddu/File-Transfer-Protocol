package main

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
)

type TCPHeader struct {
	SeqNum   int
	AckNum   int
	Checksum uint16 // Kernel will set this if it's 0
	Payload  []byte
}

func main() {
	var c *net.UDPConn
	CONNECT := "127.0.0.1:50000"

	for {

		fmt.Print("myftp>")
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		line = line[:len(line)-1]
		if err != nil {
			log.Fatal(err)
		}
		arr := strings.Split(line, " ")
		command := arr[0]
		fileName := ""
		if len(arr) > 1 {
			fileName = arr[1]
			fileName = fileName[:len(fileName)-1]
		}
		if strings.Contains(command, "connect") {
			s, err := net.ResolveUDPAddr("udp4", CONNECT)
			c, err = net.DialUDP("udp4", nil, s)
			if err != nil {
				fmt.Println(err)
				return
			}
			defer c.Close()
			threeWayHS(c)
			continue
		}
		sendCMD(c, line)
		if command == "put" {
			put(c, fileName)
		}
		if command == "get" {
			get(c, fileName)
		}
		if strings.Contains(command, "quit") {
			c.Close()
			os.Exit(0)
		}
		if strings.Contains(command, "?") {
			printHelp()
		}
	}

}
func sendCMD(c *net.UDPConn, line string) {
	pkt := createPkt(0, []byte(line))
	encodeSend(pkt, c)
}

func createPkt(seq int, payload []byte) TCPHeader {
	packet := TCPHeader{
		SeqNum:   seq,
		AckNum:   0,
		Checksum: 0, // Kernel will set this if it's 0
		Payload:  payload,
	}
	return packet
}

func (tcp TCPHeader) String() string {
	seq := fmt.Sprint(tcp.SeqNum)
	ack := fmt.Sprint(tcp.AckNum)
	string := "TCPHeader{\n\tseq:" + seq + "\n\t" + "Ack:" + ack + "\n}"
	return string

}

func TCPSender(pkt_list []TCPHeader, c *net.UDPConn) {
	ss_thres := 100
	cwnd := 1
	dupAck := make(map[int]int)
	index := 0

	for index < len(pkt_list) {
		cwnd_copy := float64(cwnd)
		fmt.Printf("CWND = %d\n", cwnd)
		for i := 0; i < cwnd; i++ {
			if index >= len(pkt_list) {
				break
			}
			if index == 7 {
				index += 1
				continue
			}
			ack := sendPkt(pkt_list[index], c)
			index += 1
			if _, ok := dupAck[ack]; !ok {
				dupAck[ack] = 0
			}
			dupAck[ack] += 1
			if dupAck[ack] >= 4 {
				ss_thres = int(math.Ceil(cwnd_copy / 2))
				cwnd_copy = 1
				sendPkt(pkt_list[ack], c)
			}
			if int(math.Ceil(cwnd_copy)) >= ss_thres {
				//fmt.Printf("cwnd_copy = %f , ss_thres = %d\n", cwnd_copy, ss_thres)
				cwnd_copy += float64(1 / cwnd_copy)
			} else if dupAck[ack] < 2 {
				cwnd_copy += 1
			}
		}
		cwnd = int(math.Ceil(cwnd_copy))
	}
}

func ReceiveExtractPkt(c *net.UDPConn) (TCPHeader, *net.UDPAddr) {
	var pkt TCPHeader
	var network bytes.Buffer

	buffer := make([]byte, 1024)
	// rand.Seed(int64(i))
	_, addr, err := c.ReadFromUDP(buffer)
	network.Write(buffer)
	// fmt.Printf("pkt size = %d", len(buffer))
	dec := gob.NewDecoder(&network)
	err = dec.Decode(&pkt)
	if err != nil {
		log.Fatal("decode error:", err)
	}
	return pkt, addr
}

func sendPkt(pkt TCPHeader, c *net.UDPConn) int {
	fmt.Println("Sending Packet")
	// fmt.Println(pkt.Checksum)
	fmt.Println(pkt)
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(pkt)
	if err != nil {
		log.Fatal("encode error:", err)
	}
	_, err = c.Write(network.Bytes())
	if err != nil {
		fmt.Println(err)
		return -1
	}
	rec_pkt, _ := ReceiveExtractPkt(c)
	ack := rec_pkt.AckNum
	fmt.Printf("Ack Recieved: %d\n", ack)
	return ack
}

func encodeSend(pkt TCPHeader, c *net.UDPConn) {
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(pkt)
	if err != nil {
		log.Fatal("encode error:", err)
	}
	_, err = c.Write(network.Bytes())
	if err != nil {
		log.Fatal(err)
	}
}

func createChunks(fileName string) []TCPHeader {
	pkt_list := []TCPHeader{}
	file, err := os.Open(fileName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer file.Close()
	fileInfo, _ := file.Stat()
	var fileSize int64 = fileInfo.Size()
	const fileChunk = 1 * (1 << 9)
	totalPartsNum := uint64(math.Ceil(float64(fileSize) / float64(fileChunk)))
	for i := uint64(0); i < totalPartsNum; i++ {
		partSize := int(math.Min(fileChunk, float64(fileSize-int64(i*fileChunk))))
		partBuffer := make([]byte, partSize)
		file.Read(partBuffer)
		pkt := createPkt(int(i), partBuffer)
		pkt.Checksum = calCheckSum(pkt)
		pkt_list = append(pkt_list, pkt)
	}
	return pkt_list
}

func calCheckSum(pkt TCPHeader) uint16 {
	checksum := uint16(0)
	// checksum += uint16(hash(strconv.Itoa(int(pkt.Source))))
	// checksum += uint16(hash(strconv.Itoa(int(pkt.Destination))))
	checksum += uint16(hash(strconv.Itoa(int(pkt.SeqNum))))
	checksum += uint16(hash(strconv.Itoa(int(pkt.AckNum))))
	checksum += uint16(calPayloadHash(pkt.Payload))
	return checksum
}

func calPayloadHash(payload []byte) uint32 {
	myString := string(payload[:])
	return hash(myString)
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func threeWayHS(c *net.UDPConn) {
	pkt := TCPHeader{}
	pkt.SeqNum = 0
	encodeSend(pkt, c)
	rec_pkt, _ := ReceiveExtractPkt(c)
	if pkt.SeqNum+1 != rec_pkt.SeqNum {
		fmt.Println("3 way handshake failed!!!")
		c.Close()
		os.Exit(0)
	}
	sending_pkt := TCPHeader{}
	sending_pkt.AckNum = rec_pkt.SeqNum + 1
	encodeSend(sending_pkt, c)
	fmt.Println("3-Way Handshake successful!!!")
	fmt.Println("Connection Established!!")
}

func put(c *net.UDPConn, fileName string) {
	// fileName = fileName[:len(fileName)-1]
	fmt.Println(fileName)
	packet_list := createChunks(fileName)
	TCPSender(packet_list, c)
}

//////////////////////////////////////////////////

func get(c *net.UDPConn, fileName string) {
	present_ack := 0
	store_pkt := make(map[int]TCPHeader)
	for {
		pkt, addr := ReceiveExtractPkt(c)
		fmt.Println(pkt)
		checksum := calCheckSum(pkt)
		if checksum != pkt.Checksum {
			fmt.Println("Tampered with packet")
			fmt.Printf("original checksum = %d, new checksum = %d", pkt.Checksum, checksum)
			os.Exit(0)
		}
		//addToFile(pkt)
		TCPReciever(pkt, present_ack, store_pkt, c, addr)
		const fileSize = 1 * (1 << 9)
		if len(pkt.Payload) < fileSize {
			break
		}
	}
	addToFile(store_pkt, fileName)
}

func TCPReciever(pkt TCPHeader, present_Ack int, store_pkt map[int]TCPHeader, c *net.UDPConn, addr *net.UDPAddr) {
	if int(pkt.SeqNum) == present_Ack {
		present_Ack += 1
	}
	store(store_pkt, pkt)
	fmt.Println("Sending Ack:" + strconv.Itoa(getAck(store_pkt)))
	new_pkt := TCPHeader{}
	new_pkt.AckNum = getAck(store_pkt)
	encodeSend(new_pkt, c)
	// sendAck(c, addr)
}

func store(store_pkt map[int]TCPHeader, pkt TCPHeader) {
	store_pkt[int(pkt.SeqNum)] = pkt
}

func addToFile(store_pkt map[int]TCPHeader, fileName string) {
	for i := 0; i < len(store_pkt); i++ {
		partBuffer := store_pkt[i].Payload
		//fileName := fileName[:len(fileName)-1]
		f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if _, err := f.Write(partBuffer); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}

}

func getAck(store_pkt map[int]TCPHeader) int {
	i := 0
	for {
		if _, ok := store_pkt[i]; ok {
			i += 1
		} else {
			break
		}
	}
	return i
}

func printHelp() {
	str := "Following are the commands:\nconnect\tconnect to remote myftp\nput\tsend file\nget\treceive file\nquit\texit tftp\n?\tprint help information"
	fmt.Println(str)

}
