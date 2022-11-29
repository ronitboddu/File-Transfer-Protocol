package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"log"
	"math"
	"net"
	"os"
	"strconv"
)

type TCPHeader struct {
	Source      uint16
	Destination uint16
	SeqNum      uint32
	AckNum      uint32
	Checksum    uint16 // Kernel will set this if it's 0
	Payload     []byte
}

func main() {
	packet_list := createChunks("Jarvis.txt")
	CONNECT := "127.0.0.1:50000"
	s, err := net.ResolveUDPAddr("udp4", CONNECT)
	c, err := net.DialUDP("udp4", nil, s)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer c.Close()
	TCPSender(packet_list, c)
}

// func createPktList() []TCPHeader {
// 	pkt_list := []TCPHeader{}
// 	for i := 0; i < 30; i++ {
// 		pkt := createPkt(uint32(i))
// 		pkt_list = append(pkt_list, pkt)
// 	}
// 	return pkt_list
// }

func createPkt(seq uint32, payload []byte) TCPHeader {
	packet := TCPHeader{
		Source:      0xaa47, // Random ephemeral port
		Destination: 80,
		SeqNum:      seq,
		AckNum:      0,
		Checksum:    0, // Kernel will set this if it's 0
		Payload:     payload,
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
			// To Drop packets
			if index == 7 {
				//fmt.Println("here")
				index += 1
				continue
			}
			// fmt.Println("Sending Packet:")
			// fmt.Println(pkt_list[index])
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

func sendPkt(pkt TCPHeader, c *net.UDPConn) int {
	fmt.Println("Sending Packet")
	fmt.Println(pkt.Checksum)
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
	buffer := make([]byte, 1024)
	n, _, err := c.ReadFromUDP(buffer)
	if err != nil {
		fmt.Println(err)
		return -1
	}
	ack, err := strconv.Atoi(string(buffer[0:n]))
	if err != nil {
		fmt.Println(err)
		return -1
	}
	fmt.Printf("Ack Recieved: %d\n", ack)
	return ack
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
		pkt := createPkt(uint32(i), partBuffer)
		calCheckSum(&pkt)
		pkt_list = append(pkt_list, pkt)
	}
	return pkt_list
}

func calCheckSum(pkt *TCPHeader) {
	pkt.Checksum += uint16(hash(strconv.Itoa(int(pkt.Source))))
	pkt.Checksum += uint16(hash(strconv.Itoa(int(pkt.Destination))))
	pkt.Checksum += uint16(hash(strconv.Itoa(int(pkt.SeqNum))))
	pkt.Checksum += uint16(hash(strconv.Itoa(int(pkt.AckNum))))
	pkt.Checksum += uint16(calPayloadHash(pkt.Payload))
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
