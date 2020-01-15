package tcp

import (
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/json-iterator/go"
	"github.com/satori/go.uuid"
	"github.com/yaice-rx/yaice/log"
	"github.com/yaice-rx/yaice/network"
	"github.com/yaice-rx/yaice/router"
	"github.com/yaice-rx/yaice/utils"
	"go.uber.org/zap"
	"net"
	"time"
)

type Conn struct {
	guid         string
	conn         *net.TCPConn
	receiveQueue chan network.IMessage
	sendQueue    chan []byte
	times        int64
}

func NewConn(conn *net.TCPConn) network.IConn {
	return &Conn{
		guid:         uuid.NewV4().String(),
		conn:         conn,
		receiveQueue: make(chan network.IMessage),
		sendQueue:    make(chan []byte),
		times:        time.Now().Unix(),
	}
}

func (c *Conn) startReadThread() {
	defer func() {
		ConnManagerMgr.Remove(c.guid)
	}()
	var errs error
	tempBuff := make([]byte, 0)
	readBuff := make([]byte, 1024)
	data := make([]byte, 1024)
	msgId := 0
	for {
		if c.conn == nil {
			return
		}
		//read
		n, e := c.conn.Read(readBuff)
		if e != nil {
			fmt.Printf("tcp read err"+e.Error())
			log.AppLogger.Error("tcp read err"+e.Error(),zap.String("function","network.tcp.conn.startReadThread"))
			continue
		}
		c.UpdateTime()
		//写入接收消息队列中
		dataPack := NewPacket()
		tempBuff = append(tempBuff, readBuff[:n]...)
		tempBuff, data, msgId, errs = dataPack.Unpack(tempBuff)
		if errs != nil {
			continue
		}
		if len(data) == 0 {
			continue
		}
		c.receiveQueue <- NewMessage(msgId, data, c)
	}
}

func (c *Conn) startWriteThread() {
	for {
		select {
		case data,state := <-c.sendQueue:
			if state {
				break
			}
			if c.conn == nil {
				return
			}
			if _, err := c.conn.Write(data); err != nil {
				//发送错误,将数据重新写入通道重新发送
				c.sendQueue <- data
				break
			}
		}
	}
}

func (c *Conn) Send(message proto.Message) error {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	data, err := json.Marshal(message)
	if err != nil {
		log.AppLogger.Fatal(err.Error(), zap.String("function", "Send"))
		return err
	}
	protoNumber := utils.ProtocalNumber(utils.GetProtoName(message))
	dataPack := NewPacket()
	c.sendQueue <- dataPack.Pack(NewMessage(protoNumber, data, c))
	return nil
}

func (c *Conn) GetGuid() string {
	return c.guid
}

func (c *Conn) Start() {
	go c.startReadThread()
	go c.startWriteThread()
	go func() {
		for {
			select {
			case msg,state := <-c.receiveQueue:
				if state {
					router.RouterMgr.ExecRouterFunc(msg)
				}
				break
			case <-time.After(time.Second * 300):
				//连接超时
				ConnManagerMgr.Remove(c.guid)
				c.Close()
				break
			}
		}
	}()
}

func (c *Conn) GetTimes() int64 {
	return c.times
}

func (c *Conn) UpdateTime() {
	c.times = time.Now().Unix()
}

func (c *Conn) Close() {
	close(c.sendQueue)
	close(c.receiveQueue)
}
