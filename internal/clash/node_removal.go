package clash

import "gorm.io/gorm"

// Prevent an in-flight delivery from recreating records while a node is removed.
func LockNodeRemoval() func() {
	if rt := GetRuntime(); rt != nil {
		rt.syncMu.Lock()
		return rt.syncMu.Unlock
	}
	return func() {}
}

func ForgetNodeTx(tx *gorm.DB, id int) error {
	// The upstream service also runs without the optional project tables in tests.
	if !tx.Migrator().HasTable(&NodeWeight{}) {
		return nil
	}
	key := NodeKey(id)
	if err := tx.Where("node_key = ?", key).Delete(&NodeWeight{}).Error; err != nil {
		return err
	}
	if err := tx.Where("node_key = ?", key).Delete(&Delivery{}).Error; err != nil {
		return err
	}
	var ratios []ClientRatio
	if err := tx.Find(&ratios).Error; err != nil {
		return err
	}
	for _, ratio := range ratios {
		weights, limits := ratio.Weights(), ratio.Limits()
		_, hasWeight := weights[key]
		_, hasLimit := limits[key]
		if !hasWeight && !hasLimit {
			continue
		}
		delete(weights, key)
		delete(limits, key)
		w, err := EncodeWeights(weights)
		if err != nil {
			return err
		}
		l, err := EncodeWeights(limits)
		if err != nil {
			return err
		}
		if err := tx.Model(&ratio).Updates(map[string]any{"weights_json": w, "limits_json": l}).Error; err != nil {
			return err
		}
	}
	return nil
}
