package main

import (
	"fmt"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// 这是一个使用示例，展示如何连接到 MHMQTT broker
// 注意：这需要安装 paho.mqtt.golang 客户端库

func main() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker("tcp://localhost:1883")
	opts.SetClientID("example-client")
	opts.SetKeepAlive(60 * time.Second)
	opts.SetDefaultPublishHandler(messageHandler)
	opts.SetPingTimeout(1 * time.Second)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	// 订阅主题
	if token := client.Subscribe("test/topic", 1, nil); token.Wait() && token.Error() != nil {
		log.Fatal(token.Error())
	}

	// 发布消息
	for i := 0; i < 10; i++ {
		text := fmt.Sprintf("Message %d", i)
		token := client.Publish("test/topic", 1, false, text)
		token.Wait()
		time.Sleep(1 * time.Second)
	}

	time.Sleep(5 * time.Second)

	client.Disconnect(250)
}

var messageHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	fmt.Printf("收到消息: %s from topic: %s\n", msg.Payload(), msg.Topic())
}

