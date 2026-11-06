package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/clash"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeDelete_RemovesAttachedLocalCopies(t *testing.T) {
	db := initTrafficTestDB(t)
	svc := &NodeService{}

	node := &model.Node{Name: "n1"}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	createNodeInbound(t, db, node.Id, "n1-in-443", 443)

	if err := clash.Migrate(); err != nil {
		t.Fatal(err)
	}
	var inbound model.Inbound
	db.Where("node_id = ?", node.Id).First(&inbound)
	other := model.Node{Name: "keep"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	createNodeInbound(t, db, other.Id, "keep-444", 444)
	var kept model.Inbound
	db.Where("node_id = ?", other.Id).First(&kept)
	client := model.ClientRecord{Email: "keep-client"}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{inbound.Id, kept.Id} {
		if err := db.Create(&model.ClientInbound{ClientId: client.Id, InboundId: id}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&xray.ClientTraffic{Email: client.Email, InboundId: inbound.Id, Up: 123}).Error; err != nil {
		t.Fatal(err)
	}
	key := clash.NodeKey(node.Id)
	if err := db.Create(&clash.NodeWeight{NodeKey: key, DefaultWeight: 2}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&clash.Delivery{NodeKey: key, ClientID: client.Id}).Error; err != nil {
		t.Fatal(err)
	}
	weights, _ := clash.EncodeWeights(map[string]int64{key: 2, clash.LocalNodeKey: 1})
	limits, _ := clash.EncodeWeights(map[string]int64{key: 100, clash.LocalNodeKey: 200})
	ratio := clash.ClientRatio{ClientID: client.Id, Mode: clash.ModeCustom, WeightsRaw: weights, LimitsRaw: limits}
	if err := db.Create(&ratio).Error; err != nil {
		t.Fatal(err)
	}
	var nodeCnt, ibCnt int64
	if err := db.Create(&model.NodeClientTraffic{NodeId: node.Id, Email: "gone"}).Error; err != nil {
		t.Fatalf("seed baseline: %v", err)
	}
	if err := svc.Delete(node.Id); err != nil {
		t.Fatalf("Delete (with attached inbound): %v", err)
	}
	var baseCnt int64
	db.Model(&model.Node{}).Where("id = ?", node.Id).Count(&nodeCnt)
	db.Model(&model.NodeClientTraffic{}).Where("node_id = ?", node.Id).Count(&baseCnt)
	if nodeCnt != 0 || baseCnt != 0 {
		t.Fatalf("after delete: node=%d baseline=%d, want 0/0", nodeCnt, baseCnt)
	}
	db.Model(&model.Inbound{}).Where("node_id = ?", node.Id).Count(&ibCnt)
	if ibCnt != 0 {
		t.Fatalf("remaining inbounds: %d", ibCnt)
	}
	// A snapshot fetched before deletion must not adopt the inbound again.
	syncNode(t, &InboundService{}, node.Id, "n1-in-443")
	db.Model(&model.Inbound{}).Where("node_id = ?", node.Id).Count(&ibCnt)
	if ibCnt != 0 {
		t.Fatal("late snapshot recreated deleted inbound")
	}

	var count int64
	for _, check := range []struct {
		table any
		query string
		arg   any
		want  int64
	}{
		{&model.Node{}, "id = ?", other.Id, 1},
		{&model.Inbound{}, "id = ?", kept.Id, 1},
		{&model.ClientRecord{}, "id = ?", client.Id, 1},
		{&model.ClientInbound{}, "inbound_id = ?", kept.Id, 1},
		{&model.ClientInbound{}, "inbound_id = ?", inbound.Id, 0},
		{&clash.NodeWeight{}, "node_key = ?", key, 0},
		{&clash.Delivery{}, "node_key = ?", key, 0},
	} {
		if err := db.Model(check.table).Where(check.query, check.arg).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%T count=%d want=%d", check.table, count, check.want)
		}
	}
	traffic := readTraffic(t, db, client.Email)
	if traffic.Up != 123 || traffic.InboundId != 0 {
		t.Fatalf("traffic not preserved/detached: %+v", traffic)
	}
	if err := db.First(&ratio, ratio.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, ok := ratio.Limits()[key]; ok {
		t.Fatal("deleted-node limit remains")
	}
	if _, ok := ratio.Weights()[key]; ok {
		t.Fatal("deleted-node weight remains")
	}
	if ratio.Limits()[clash.LocalNodeKey] != 200 || ratio.Weights()[clash.LocalNodeKey] != 1 {
		t.Fatal("other-node settings changed")
	}

}
