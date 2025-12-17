package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mhmqtt/internal/broker"
	"mhmqtt/internal/config"
	"mhmqtt/internal/httpapi"
)

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	flag.Parse()

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 创建 broker
	b := broker.NewBroker(cfg)
	if err := b.Start(); err != nil {
		log.Fatalf("启动 broker 失败: %v", err)
	}
	defer b.Stop()

	// 启动 HTTP API
	api := httpapi.NewAPI(b, cfg)
	go func() {
		if err := api.Start(); err != nil {
			log.Fatalf("启动 HTTP API 失败: %v", err)
		}
	}()

	log.Printf("MQTT Broker 启动成功")
	log.Printf("MQTT 监听地址: %s", cfg.Broker.Address)
	log.Printf("WebSocket 监听地址: %s", cfg.Broker.WSAddress)
	log.Printf("HTTP API 监听地址: %s", cfg.Broker.HTTPAddress)

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("正在关闭 broker...")
}

