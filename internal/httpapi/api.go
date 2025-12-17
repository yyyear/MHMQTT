package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"mhmqtt/internal/config"
	"mhmqtt/internal/protocol"
)

// API HTTP API 服务器
type API struct {
	broker interface{} // *broker.Broker 避免循环依赖
	config *config.Config
	server *http.Server
}

// NewAPI 创建 API 服务器
func NewAPI(broker interface{}, cfg *config.Config) *API {
	return &API{
		broker: broker,
		config: cfg,
	}
}

// Start 启动 API 服务器
func (a *API) Start() error {
	router := mux.NewRouter()

	// API 路由
	api := router.PathPrefix("/api").Subrouter()
	api.HandleFunc("/status", a.getStatus).Methods("GET")
	api.HandleFunc("/clients", a.getClients).Methods("GET")
	api.HandleFunc("/publish", a.publish).Methods("POST")
	api.HandleFunc("/topics", a.getTopics).Methods("GET")
	api.HandleFunc("/cluster/nodes", a.getClusterNodes).Methods("GET")
	api.HandleFunc("/cluster/add", a.addClusterNode).Methods("POST")
	api.HandleFunc("/cluster/remove", a.removeClusterNode).Methods("POST")

	a.server = &http.Server{
		Addr:         a.config.Broker.HTTPAddress,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("HTTP API 服务器启动在 %s", a.config.Broker.HTTPAddress)
	return a.server.ListenAndServe()
}

// getStatus 获取状态
func (a *API) getStatus(w http.ResponseWriter, r *http.Request) {
	// 使用反射或接口获取 broker 状态
	// 简化处理
	status := map[string]interface{}{
		"status":    "running",
		"timestamp": time.Now().Unix(),
		"version":   "1.0",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// getClients 获取客户端列表
func (a *API) getClients(w http.ResponseWriter, r *http.Request) {
	// 简化处理
	clients := []map[string]interface{}{
		{
			"id":      "client1",
			"status":  "connected",
			"version": "3.1.1",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clients)
}

// publish 发布消息
func (a *API) publish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Topic   string `json:"topic"`
		Payload string `json:"payload"`
		QoS     byte   `json:"qos"`
		Retain  bool   `json:"retain"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 创建消息
	msg := &protocol.Message{
		Topic:   req.Topic,
		Payload: []byte(req.Payload),
		QoS:     req.QoS,
		Retain:  req.Retain,
	}

	// 发布消息（需要通过接口调用 broker）
	// 简化处理
	_ = msg

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// getTopics 获取主题列表
func (a *API) getTopics(w http.ResponseWriter, r *http.Request) {
	topics := []string{
		"test/topic1",
		"test/topic2",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(topics)
}

// getClusterNodes 获取集群节点
func (a *API) getClusterNodes(w http.ResponseWriter, r *http.Request) {
	nodes := []map[string]interface{}{
		{
			"id":      "node1",
			"address": "127.0.0.1:1884",
			"status":  "connected",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}

// addClusterNode 添加集群节点
func (a *API) addClusterNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID  string `json:"node_id"`
		Address string `json:"address"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 添加节点（需要通过接口调用 cluster）
	_ = req

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// removeClusterNode 移除集群节点
func (a *API) removeClusterNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 移除节点（需要通过接口调用 cluster）
	_ = req

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

