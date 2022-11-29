package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
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
	present_ack := 0
	store_pkt := make(map[int]TCPHeader)
	for {
		var pkt TCPHeader
		var network bytes.Buffer

		buffer := make([]byte, 1024)
		// rand.Seed(int64(i))
		_, addr, err := connection.ReadFromUDP(buffer)
		network.Write(buffer)
		// fmt.Printf("pkt size = %d", len(buffer))
		dec := gob.NewDecoder(&network)
		err = dec.Decode(&pkt)
		if err != nil {
			log.Fatal("decode error:", err)
		}
		fmt.Println(pkt)
		checksum := calCheckSum(pkt)
		if checksum != pkt.Checksum {
			fmt.Println("Tampered with packet")
			fmt.Printf("original checksum = %d, new checksum = %d", pkt.Checksum, checksum)
			os.Exit(0)
		}
		//addToFile(pkt)
		TCPReciever(pkt, present_ack, store_pkt, connection, addr)
		const fileSize = 1 * (1 << 9)
		if len(pkt.Payload) < fileSize {
			break
		}
	}
	addToFile(store_pkt)
}

func TCPReciever(pkt TCPHeader, present_Ack int, store_pkt map[int]TCPHeader, c *net.UDPConn, addr *net.UDPAddr) {
	if int(pkt.SeqNum) == present_Ack {
		present_Ack += 1
	}
	store(store_pkt, pkt)
	fmt.Println("Sending Ack:" + string(getAck(store_pkt)))
	sendAck(c, addr, getAck(store_pkt))
}

func sendAck(c *net.UDPConn, addr *net.UDPAddr, data []byte) {
	_, err := c.WriteToUDP(data, addr)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func store(store_pkt map[int]TCPHeader, pkt TCPHeader) {
	store_pkt[int(pkt.SeqNum)] = pkt
}

func getAck(store_pkt map[int]TCPHeader) []byte {
	i := 0
	for {
		if _, ok := store_pkt[i]; ok {
			i += 1
		} else {
			break
		}
	}
	return []byte(strconv.Itoa(i))
}

func (tcp TCPHeader) String() string {
	seq := fmt.Sprint(tcp.SeqNum)
	ack := fmt.Sprint(tcp.AckNum)
	string := "TCPHeader{\n\tseq:" + seq + "\n\t" + "Ack:" + ack + "\n}"
	return string

}

func addToFile(store_pkt map[int]TCPHeader) {
	for i := 0; i < len(store_pkt); i++ {
		partBuffer := store_pkt[i].Payload
		fileName := "NewFile.txt"
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
	checksum += uint16(hash(strconv.Itoa(int(pkt.Source))))
	checksum += uint16(hash(strconv.Itoa(int(pkt.Destination))))
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
