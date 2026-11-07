package subscription

import (
	"errors"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/policy"
	"github.com/mhsanaei/3x-ui/v3/internal/clash/storage"
)

type TemplateGroupInjection struct {
	Group    string   `json:"group"`
	Nodes    []string `json:"nodes"`
	AllNodes bool     `json:"allNodes"`
}
type TemplateRouteInjection struct {
	RuleSetID uint   `json:"ruleSetId"`
	Target    string `json:"target"`
}
type TemplateBlacklistInjection struct {
	RuleSetID uint   `json:"ruleSetId"`
	Node      string `json:"node"`
}
type TemplateInjections struct {
	Groups     []TemplateGroupInjection     `json:"groups"`
	Routes     []TemplateRouteInjection     `json:"routes"`
	Blacklists []TemplateBlacklistInjection `json:"blacklists"`
}
type YAMLTemplate struct {
	Injections TemplateInjections `json:"injections" gorm:"serializer:json"`

	ID      uint     `json:"id"`
	Name    string   `json:"name" gorm:"uniqueIndex;not null"`
	Content string   `json:"content"`
	Groups  []string `json:"groups" gorm:"-"`
}

func (YAMLTemplate) TableName() string { return "clash_yaml_templates" }

func (t *YAMLTemplate) parse() error {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(t.Content), &config); err != nil {
		return err
	}
	if len(config) == 0 {
		return errors.New("模板必须是非空 YAML 配置对象")
	}
	t.Groups = []string{}
	if _, ok := config["proxy-groups"]; !ok {
		return nil
	}
	var parsed struct {
		Groups []struct{ Name string } `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal([]byte(t.Content), &parsed); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, g := range parsed.Groups {
		if strings.TrimSpace(g.Name) == "" || seen[g.Name] {
			return errors.New("策略组名称不能为空或重复")
		}
		seen[g.Name] = true
		t.Groups = append(t.Groups, g.Name)
	}
	return nil
}

func Templates() ([]YAMLTemplate, error) {
	db, err := storage.DB()
	if err != nil {
		return nil, err
	}
	var rows []YAMLTemplate
	if err := db.Order("id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if err := rows[i].parse(); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
func SaveTemplate(t *YAMLTemplate) error {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("请填写模板名称")
	}
	if err := t.parse(); err != nil {
		return err
	}
	var config map[string]any
	if err := yaml.Unmarshal([]byte(t.Content), &config); err != nil {
		return err
	}
	groups, err := subscriptionGroups(config)
	if err != nil {
		return err
	}
	if err := validateSubscriptionGraph(groups); err != nil {
		return err
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	cat, err := policy.LoadCatalogOrError()
	if err != nil {
		return err
	}
	nodeKeys := map[string]bool{}
	for _, node := range cat.Nodes {
		nodeKeys[node.Key] = true
	}
	for _, binding := range t.Injections.Groups {
		if binding.AllNodes && len(binding.Nodes) > 0 {
			return errors.New("全部节点与指定节点不能同时选择")
		}
		for _, key := range binding.Nodes {
			if !nodeKeys[key] {
				return errors.New("注入节点不存在")
			}
		}
	}
	for _, binding := range t.Injections.Blacklists {
		if !nodeKeys[binding.Node] {
			return errors.New("黑名单节点不存在")
		}
	}
	targets := map[string]bool{"DIRECT": true, "REJECT": true}
	for _, name := range t.Groups {
		targets[name] = true
	}
	seen := map[string]bool{}
	for _, binding := range t.Injections.Groups {
		if !targets[binding.Group] || binding.Group == "DIRECT" || binding.Group == "REJECT" || seen[binding.Group] {
			return errors.New("注入策略组不存在或重复")
		}
		seen[binding.Group] = true
	}
	for _, binding := range t.Injections.Routes {
		var rule RuleSet
		if err := db.First(&rule, binding.RuleSetID).Error; err != nil {
			return err
		}
		if !rule.Routing || !targets[binding.Target] {
			return errors.New("规则集路由用途或目标无效")
		}
	}
	for _, binding := range t.Injections.Blacklists {
		var rule RuleSet
		if err := db.First(&rule, binding.RuleSetID).Error; err != nil {
			return err
		}
		if !rule.Blacklist || binding.Node == "" {
			return errors.New("黑名单用途或节点无效")
		}
	}
	if t.ID == 0 {
		return db.Create(t).Error
	}
	res := db.Model(t).Select("Name", "Content", "Injections").Updates(t)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("模板不存在")
	}
	return nil
}

func DeleteTemplate(id uint) error {
	if id == 0 {
		return errors.New("模板 ID 无效")
	}
	db, err := storage.DB()
	if err != nil {
		return err
	}
	var references int64
	if err := db.Model(&SubscriptionBinding{}).Where("template_id = ?", id).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return errors.New("模板已绑定客户端订阅，请先解除绑定")
	}
	result := db.Delete(&YAMLTemplate{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("模板不存在")
	}
	return nil
}
