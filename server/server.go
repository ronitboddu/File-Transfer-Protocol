package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"math/rand"
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

func random(min, max int) int {
	return rand.Intn(max-min) + min
}

func main() {

	arguments := os.Args
	if len(arguments) == 1 {
		fmt.Println("Please provide a port number!")
		return
	}
	PORT := ":" + arguments[1]

	s, err := net.ResolveUDPAddr("udp4", PORT)
	if err != nil {
		fmt.Println(err)
		return
	}

	connection, err := net.ListenUDP("udp4", s)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer connection.Close()
	threeWayHS(connection)
	for {
		cmd, fileName, addr := receiveCMD(connection)
		fmt.Println(cmd)
		if cmd == "put" {
			put(connection, fileName)
		}
		if cmd == "get" {
			get(connection, fileName, addr)
		}
		if strings.Contains(cmd, "quit") {
			connection.Close()
			os.Exit(0)
		}
		if strings.Contains(cmd, "?") {
			continue
		}
	}

}

func receiveCMD(c *net.UDPConn) (string, string, *net.UDPAddr) {
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
	line := string(pkt.Payload)
	arr := strings.Split(line, " ")
	var fileName string
	cmd := arr[0]
	if len(arr) > 1 {
		fileName = arr[1]
	}
	return cmd, fileName, addr
}

func put(c *net.UDPConn, fileName string) {
	present_ack := 0
	store_pkt := make(map[int]TCPHeader)
	for {
		pkt, addr := RecieveExtractPkt(c)
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
	encodeSend(new_pkt, c, addr)
	// sendAck(c, addr)
}

func sendAck(c *net.UDPConn, addr *net.UDPAddr, data []byte) {
	_, err := c.WriteToUDP(data, addr)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func RecieveExtractPkt(c *net.UDPConn) (TCPHeader, *net.UDPAddr) {
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

func store(store_pkt map[int]TCPHeader, pkt TCPHeader) {
	store_pkt[int(pkt.SeqNum)] = pkt
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

func (tcp TCPHeader) String() string {
	seq := fmt.Sprint(tcp.SeqNum)
	ack := fmt.Sprint(tcp.AckNum)
	string := "TCPHeader{\n\tseq:" + seq + "\n\t" + "Ack:" + ack + "\n}"
	return string

}

func addToFile(store_pkt map[int]TCPHeader, fileName string) {
	for i := 0; i < len(store_pkt); i++ {
		partBuffer := store_pkt[i].Payload
		fileName := fileName[:len(fileName)-1]
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

func encodeSend(pkt TCPHeader, c *net.UDPConn, addr *net.UDPAddr) {
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(pkt)
	if err != nil {
		log.Fatal("encode error:", err)
	}
	_, err = c.WriteToUDP(network.Bytes(), addr)
	if err != nil {
		log.Fatal(err)
	}
}

func threeWayHS(c *net.UDPConn) {
	pkt, addr := RecieveExtractPkt(c)
	new_pkt := TCPHeader{}
	new_pkt.SeqNum = 1
	new_pkt.AckNum = pkt.SeqNum + 1
	encodeSend(new_pkt, c, addr)
	rec_pkt, _ := RecieveExtractPkt(c)
	if rec_pkt.AckNum != new_pkt.SeqNum+1 {
		fmt.Println("3 way Handshake failed!!!!")
		fmt.Printf("RSeq = %d , NSeq = %d", rec_pkt.SeqNum, new_pkt.SeqNum+1)
		c.Close()
		os.Exit(0)
	}
	fmt.Println("3-Way Handshake successful!!!")
	fmt.Println("Connection Established!!")
}

////////////////////////////////////////////////////////////////////////////////////

func createPkt(seq int, payload []byte) TCPHeader {
	packet := TCPHeader{
		SeqNum:   seq,
		AckNum:   0,
		Checksum: 0, // Kernel will set this if it's 0
		Payload:  payload,
	}
	return packet
}

func createChunks(fileName string) []TCPHeader {
	pkt_list := []TCPHeader{}
	fmt.Println(fileName[:len(fileName)-1])
	fileName = fileName[:len(fileName)-1]
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

func get(c *net.UDPConn, fileName string, addr *net.UDPAddr) {
	packet_list := createChunks(fileName)
	TCPSender(packet_list, c, addr)
}

func TCPSender(pkt_list []TCPHeader, c *net.UDPConn, addr *net.UDPAddr) {
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
			ack := sendPkt(pkt_list[index], c, addr)
			index += 1
			if _, ok := dupAck[ack]; !ok {
				dupAck[ack] = 0
			}
			dupAck[ack] += 1
			if dupAck[ack] >= 4 {
				ss_thres = int(math.Ceil(cwnd_copy / 2))
				cwnd_copy = 1
				sendPkt(pkt_list[ack], c, addr)
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

func sendPkt(pkt TCPHeader, c *net.UDPConn, addr *net.UDPAddr) int {
	fmt.Println("Sending Packet")
	fmt.Println(pkt.Checksum)
	fmt.Println(pkt)
	var network bytes.Buffer
	enc := gob.NewEncoder(&network)
	err := enc.Encode(pkt)
	if err != nil {
		log.Fatal("encode error:", err)
	}
	_, err = c.WriteToUDP(network.Bytes(), addr)
	if err != nil {
		fmt.Println(err)
		return -1
	}
	rec_pkt, _ := RecieveExtractPkt(c)
	ack := rec_pkt.AckNum
	fmt.Printf("Ack Recieved: %d\n", ack)
	return ack
}
