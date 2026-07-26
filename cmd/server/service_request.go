package main

import "strings"

// ServiceRequestCatalogItem is one entry in the service-request catalog.
type ServiceRequestCatalogItem struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Priority    string `json:"priority,omitempty"` // default priority when creating
}

var defaultServiceRequestCatalog = []ServiceRequestCatalogItem{
	{ID: "account_provision", Category: "账号", Title: "账号开通", Description: "新建业务/运维账号", Priority: "p3"},
	{ID: "permission_grant", Category: "权限", Title: "权限申请", Description: "系统/数据权限开通或变更", Priority: "p3"},
	{ID: "capacity_scale", Category: "容量", Title: "容量扩容", Description: "主机/磁盘/实例扩容", Priority: "p2"},
	{ID: "network_access", Category: "网络", Title: "网络策略开通", Description: "防火墙/安全组/访问白名单", Priority: "p2"},
	{ID: "cert_renew", Category: "证书", Title: "证书申请/续期", Description: "TLS/服务证书", Priority: "p3"},
	{ID: "db_access", Category: "数据库", Title: "数据库访问申请", Description: "只读账号或连接授权", Priority: "p3"},
	{ID: "deploy_assist", Category: "发布", Title: "发布协助", Description: "非标准发布窗口协助", Priority: "p2"},
	{ID: "other_sr", Category: "其他", Title: "其他服务请求", Description: "未归类的服务台请求", Priority: "p4"},
}

func (cs *ConfigStore) ServiceRequestCatalog() []ServiceRequestCatalogItem {
	cs.mu.RLock()
	custom := append([]ServiceRequestCatalogItem{}, cs.cfg.ServiceRequestCatalog...)
	cs.mu.RUnlock()
	if len(custom) == 0 {
		out := make([]ServiceRequestCatalogItem, len(defaultServiceRequestCatalog))
		copy(out, defaultServiceRequestCatalog)
		return out
	}
	return custom
}

func (cs *ConfigStore) FindServiceRequestCatalogItem(id string) (ServiceRequestCatalogItem, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ServiceRequestCatalogItem{}, false
	}
	for _, it := range cs.ServiceRequestCatalog() {
		if it.ID == id {
			return it, true
		}
	}
	return ServiceRequestCatalogItem{}, false
}
