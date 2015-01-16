package netutils

import (
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	//"sync/atomic"
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
		sock, err := net.DialTCP("tcp", ladr, radr)
		if err != nil {
			time.Sleep(time.Duration(20+rand.Intn(15)) * time.Second)
			continue
		}
		//testing health of the new socket. GO sometimes doesnt rise the error when
		// we receive RST from remote side
		_, err = sock.Write(init_msg)
		if err != nil {
			sock.Close()
			time.Sleep(time.Duration(20+rand.Intn(15)) * time.Second)
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

/* --------------------- CONNECTION MANAGER -------------------------
Connection manager will allow send data and receive data from remote hosts.
it will have single ConnectionMsg (see below) read chan and single
ConnectionMsg write chan toward it's clients
as well as single read chan from sockets, but multiple write sockets.
it will route msgs according to Host field in connectionManager struct
(if it received from client, it will send this msg toward Host's sockets;
if recved from socket, will proxy it toward client(and client will know from which remote host
it was received)
-------------------------------------------------------------------- */
/*
MsgType's could be:
from Api's client to ConnectionManager:
"Data" - msg with Data to Host
"Connect" - connect to new Host
...
from ConnectionManager to Api's client:
"BufferFlush" - notification that connection to remote Host not longer working.
advice to flush all the msg buffers assosiated with remote host
*/
type ConnectionMsg struct {
	Host string
	Data []byte
	Type string
}

/*
Receive msg from tcp socket and send it as a ConnectionMsg to readChan
TODO: think about more generic version to be more DRYer (to work in both CM and []byte chans
*/
func CMReadFromTCP(sock *net.TCPConn, readChan chan ConnectionMsg,
	peerAddress string) {
	msgBuf := make([]byte, 65000)
	loop := 1
	var msg ConnectionMsg
	msg.Host = peerAddress
	msg.Type = "Data"
	for loop == 1 {
		bytes, err := sock.Read(msgBuf)
		if err != nil {
			msg.Type = "ReadError"
			readChan <- msg
			loop = 0
			continue
		}
		msg.Data = msgBuf[:bytes]
		readChan <- msg
	}
}

/*
ConnectionManager write instance to tcp w/ erorr propagation to/from "read" part of the socket
TODO: think about more generic version to be more DRYer (to work in both CM and []byte chans
*/
func CMWriteToTCP(sock *net.TCPConn, writeChan, readChan chan ConnectionMsg,
	peerAddress string) {
	loop := 1
	var errorMsg ConnectionMsg
	errorMsg.Host = peerAddress
	errorMsg.Type = "WriteError"
	for loop == 1 {
		select {
		case msg := <-writeChan:
			switch msg.Type {
			case "Data":
				_, err := sock.Write(msg.Data)
				if err != nil {
					for loop == 1 {
						select {
						case readChan <- errorMsg:
						case errorMsg := <-writeChan:
							if errorMsg.Type != "ConnectionError" {
								continue
							}
						}
						loop = 0
					}
				}
			case "ConnectionError":
				loop = 0
			default:
				continue
			}
		}
	}
}

func StartConnection(tcpConn *net.TCPConn, writeChan,
	readChan chan ConnectionMsg, peerAddress string) {
	go CMReadFromTCP(tcpConn, readChan, peerAddress)
	go CMWriteToTCP(tcpConn, writeChan, readChan, peerAddress)
}

func CMListenForConnection(mutex *sync.RWMutex, localPort int,
	writeChanMap map[string]chan ConnectionMsg,
	connectionStateMap map[string]int,
	readChan chan ConnectionMsg) {
	laddr := strings.Join([]string{":", strconv.Itoa(localPort)}, "")
	tcpLaddr, err := net.ResolveTCPAddr("tcp", laddr)
	if err != nil {
		panic("cant resolve local address for binding")
	}
	tcpListener, err := net.ListenTCP("tcp", tcpLaddr)
	if err != nil {
		panic("cant listen on local address for binding")
	}
	for {
		tcpConn, err := tcpListener.AcceptTCP()
		if err == nil {
			radr := strings.Split(tcpConn.RemoteAddr().String(), ":")[0]
			// check if we already has connection to remote peer as a client
			mutex.Lock()
			if val, exist := connectionStateMap[radr]; exist && val == 1 {
				tcpConn.Close()
				mutex.Unlock()
				continue
			}
			connectionStateMap[radr] = 1
			if writeChan, exist := writeChanMap[radr]; exist {
				mutex.Unlock()
				go StartConnection(tcpConn, writeChan, readChan, radr)
			} else {
				writeChanMap[radr] = make(chan ConnectionMsg)
				mutex.Unlock()
				go StartConnection(tcpConn, writeChanMap[radr], readChan, radr)
			}
		}
	}
}

func CMConnectToRemotePeer(mutex *sync.RWMutex, peerTcpAddr *net.TCPAddr,
	radr string,
	writeChan chan ConnectionMsg,
	readChan chan ConnectionMsg,
	connectionStateMap map[string]int) {
	connectLoop := 1
	for connectLoop == 1 {
		mutex.RLock()
		if connectionStateMap[radr] == 1 {
			connectLoop = 0
			mutex.RUnlock()
			continue
		}
		mutex.RUnlock()
		tcpConn, err := net.DialTCP("tcp", nil, peerTcpAddr)
		if err != nil {
			time.Sleep(time.Second * time.Duration(rand.Int63n(15)))
			continue
		}
		mutex.Lock()
		if connectionStateMap[radr] == 0 {
			connectionStateMap[radr] = 1
			go StartConnection(tcpConn, writeChan, readChan, radr)
			mutex.Unlock()
			connectLoop = 0
			continue
		} else {
			mutex.Unlock()
			tcpConn.Close()
			connectLoop = 0
			continue
		}
	}
}

func ConnectionManager(msgChan chan ConnectionMsg, localPort int) {
	writeChanMap := make(map[string]chan ConnectionMsg)
	connectionStateMap := make(map[string]int)
	readChan := make(chan ConnectionMsg)
	var connectionMutex sync.RWMutex
	go CMListenForConnection(&connectionMutex, localPort, writeChanMap,
		connectionStateMap, readChan)
	for {
		select {
		case msgToPeer := <-msgChan:
			switch msgToPeer.Type {
			case "Data":
				if state, exists := connectionStateMap[msgToPeer.Host]; exists && state == 1 {
					/*FIXME/THINK: There could be deadlock if connectin closes before we will
					be able to send to the chan */
					writeChan := writeChanMap[msgToPeer.Host]
					writeChan <- msgToPeer
				} else {
					msgChan <- ConnectionMsg{Type: "ConnectionNotExist"}
				}
			case "Connect":
				if len(strings.Split(msgToPeer.Host, ":")) > 1 {
					radr := strings.Split(msgToPeer.Host, ":")[0]
					connectionMutex.Lock()
					if _, exist := writeChanMap[radr]; !exist {
						writeChanMap[radr] = make(chan ConnectionMsg)
					}
					connectionMutex.Unlock()
					peerTcpAddr, err := net.ResolveTCPAddr("tcp", msgToPeer.Host)
					if err != nil {
						//XXX: think about , mb make something less drastic
						panic("cant resolve remote address")
					}
					go CMConnectToRemotePeer(&connectionMutex, peerTcpAddr, radr,
						writeChanMap[radr], readChan, connectionStateMap)
				} else {
					connectionMutex.Lock()
					if _, exist := writeChanMap[msgToPeer.Host]; !exist {
						writeChanMap[msgToPeer.Host] = make(chan ConnectionMsg)
					}
					connectionMutex.Unlock()
					remoteAddr := strings.Join([]string{msgToPeer.Host, strconv.Itoa(localPort)}, ":")
					peerTcpAddr, err := net.ResolveTCPAddr("tcp", remoteAddr)
					if err != nil {
						//XXX: again panic could be overkill
						panic("cant resolve remote address")
					}
					go CMConnectToRemotePeer(&connectionMutex, peerTcpAddr, msgToPeer.Host,
						writeChanMap[msgToPeer.Host], readChan, connectionStateMap)
				}
			}
		case msgFromPeer := <-readChan:
			switch msgFromPeer.Type {
			case "Data":
				msgChan <- msgFromPeer
			case "WriteError", "ReadError":
				connectionMutex.Lock()
				connectionStateMap[msgFromPeer.Host] = 0
				connectionMutex.Unlock()
				if msgFromPeer.Type == "ReadError" {
					writeChanMap[msgFromPeer.Host] <- ConnectionMsg{Type: "ConnectionError"}
				}
				var msgToApiClient ConnectionMsg
				msgToApiClient.Host = msgFromPeer.Host
				msgToApiClient.Type = "BufferFlush"
				msgChan <- msgToApiClient

			}
		}
	}
}