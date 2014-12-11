package utils

import (
	"math/rand"
	"net"
	"time"
)

//Receive msg from tcp socket and send it as a []byte to readChan
func ReadFromTCP(sock *net.TCPConn, msgBuf []byte, readChan chan []byte,
	feedbackChanFromSocket chan int) {
	loop := 1
	for loop == 1 {
		bytes, err := sock.Read(msgBuf)
		if err != nil {
			feedbackChanFromSocket <- 1
			loop = 0
			continue
		}
		readChan <- msgBuf[:bytes]
	}
}

/*simple write to TCP, for oneway connections only (no communitcation w/ "read" part of the socket
in terms of error propogation)*/
func WriteToTCPw(sock *net.TCPConn, writeChan chan []byte,
	feedbackChan chan int) {
	loop := 1
	for loop == 1 {
		select {
		case msg := <-writeChan:
			_, err := sock.Write(msg)
			if err != nil {
				feedbackChan <- 1
				continue
			}
		case <-feedbackChan:
			loop = 0
		}
	}
}

//simple write to tcp w/ erorr propagation to/from "read" part of the socket
func WriteToTCPrw(sock *net.TCPConn, writeChan chan []byte,
	feedbackChanFromSocket, feedbackChanToSocket chan int) {
	loop := 1
	for loop == 1 {
		select {
		case msg := <-writeChan:
			_, err := sock.Write(msg)
			if err != nil {
				select {
				case feedbackChanFromSocket <- 1:
					continue
				case loop = <-feedbackChanToSocket:
					loop = 0
					continue
				}
			}
		case <-feedbackChanToSocket:
			loop = 0
		}
	}
}

//reconnecting to remote host for both read and write purpose
func ReconnectTCPRW(ladr, radr *net.TCPAddr, msgBuf []byte, writeChan chan []byte,
	readChan chan []byte, feedbackChanToSocket, feedbackChanFromSocket chan int,
	init_msg []byte) {
	loop := 1
	for loop == 1 {
		time.Sleep(time.Duration(20+rand.Intn(15)) * time.Second)
		sock, err := net.DialTCP("tcp", ladr, radr)
		if err != nil {
			continue
		}
		//testing health of the new socket. GO sometimes doesnt rise the error when
		// we receive RST from remote side
		_, err = sock.Write(init_msg)
		if err != nil {
			sock.Close()
			continue
		}
		loop = 0
		go ReadFromTCP(sock, msgBuf, readChan, feedbackChanFromSocket)
		go WriteToTCPrw(sock, writeChan, feedbackChanFromSocket, feedbackChanToSocket)
	}
}

func AutoRecoonectedTCP(ladr, radr *net.TCPAddr, msgBuf, initMsg []byte,
	writeChan, readChan chan []byte, flushChan chan int) {
	feedbackChanFromSocket := make(chan int)
	feedbackChanToSocket := make(chan int)
	go ReconnectTCPRW(ladr, radr, msgBuf, writeChan, readChan, feedbackChanToSocket,
		feedbackChanFromSocket, initMsg)
	for {
		select {
		case feedbackFromSocket := <-feedbackChanFromSocket:
			feedbackChanToSocket <- feedbackFromSocket
			flushChan <- 1
			go ReconnectTCPRW(ladr, radr, msgBuf, writeChan,
				readChan, feedbackChanToSocket,
				feedbackChanFromSocket, initMsg)

		}
	}

}

//reconnecting to remote host for write only
func ReconnectTCPW(radr net.TCPAddr, writeChan chan []byte, feedbackChan chan int) {
	loop := 1
	for loop == 1 {
		time.Sleep(time.Duration(20+rand.Intn(15)) * time.Second)
		sock, err := net.DialTCP("tcp", nil, &radr)
		if err != nil {
			continue
		}
		//testing health of the new socket. GO sometimes doesnt rise the error when
		// we receive RST from remote side
		_, err = sock.Write([]byte{1})
		if err != nil {
			sock.Close()
			continue
		}
		loop = 0
		go WriteToTCPw(sock, writeChan, feedbackChan)
	}
}
