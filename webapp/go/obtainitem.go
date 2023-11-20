package main

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// obtainItem アイテム付与処理
func (h *Handler) obtainItem(tx *sqlx.Tx, userID, itemID int64, itemType int, obtainAmount int64, requestAt int64) ([]int64, []*UserCard, []*UserItem, error) {
	obtainCoins := make([]int64, 0)
	obtainCards := make([]*UserCard, 0)
	obtainItems := make([]*UserItem, 0)

	switch itemType {
	case 1: // coin
		user := new(User)
		query := "SELECT * FROM users WHERE id=?"
		if err := tx.Get(user, query, userID); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, nil, ErrUserNotFound
			}
			return nil, nil, nil, err
		}

		query = "UPDATE users SET isu_coin=? WHERE id=?"
		totalCoin := user.IsuCoin + obtainAmount
		if _, err := tx.Exec(query, totalCoin, user.ID); err != nil {
			return nil, nil, nil, err
		}
		obtainCoins = append(obtainCoins, obtainAmount)

	case 2: // card(ハンマー)
		query := "SELECT * FROM item_masters WHERE id=? AND item_type=?"
		item := new(ItemMaster)
		if err := tx.Get(item, query, itemID, itemType); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, nil, ErrItemNotFound
			}
			return nil, nil, nil, err
		}

		cID, err := h.generateID()
		if err != nil {
			return nil, nil, nil, err
		}
		card := &UserCard{
			ID:           cID,
			UserID:       userID,
			CardID:       item.ID,
			AmountPerSec: *item.AmountPerSec,
			Level:        1,
			TotalExp:     0,
			CreatedAt:    requestAt,
			UpdatedAt:    requestAt,
		}
		query = "INSERT INTO user_cards(id, user_id, card_id, amount_per_sec, level, total_exp, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"
		if _, err := tx.Exec(query, card.ID, card.UserID, card.CardID, card.AmountPerSec, card.Level, card.TotalExp, card.CreatedAt, card.UpdatedAt); err != nil {
			return nil, nil, nil, err
		}
		obtainCards = append(obtainCards, card)

	case 3, 4: // 強化素材
		query := "SELECT * FROM item_masters WHERE id=? AND item_type=?"
		item := new(ItemMaster)
		if err := tx.Get(item, query, itemID, itemType); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil, nil, ErrItemNotFound
			}
			return nil, nil, nil, err
		}

		query = "SELECT * FROM user_items WHERE user_id=? AND item_id=?"
		uitem := new(UserItem)
		if err := tx.Get(uitem, query, userID, item.ID); err != nil {
			if err != sql.ErrNoRows {
				return nil, nil, nil, err
			}
			uitem = nil
		}

		if uitem == nil {
			uitemID, err := h.generateID()
			if err != nil {
				return nil, nil, nil, err
			}
			uitem = &UserItem{
				ID:        uitemID,
				UserID:    userID,
				ItemType:  item.ItemType,
				ItemID:    item.ID,
				Amount:    int(obtainAmount),
				CreatedAt: requestAt,
				UpdatedAt: requestAt,
			}
			query = "INSERT INTO user_items(id, user_id, item_id, item_type, amount, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)"
			if _, err := tx.Exec(query, uitem.ID, userID, uitem.ItemID, uitem.ItemType, uitem.Amount, requestAt, requestAt); err != nil {
				return nil, nil, nil, err
			}

		} else {
			uitem.Amount += int(obtainAmount)
			uitem.UpdatedAt = requestAt
			query = "UPDATE user_items SET amount=?, updated_at=? WHERE id=?"
			if _, err := tx.Exec(query, uitem.Amount, uitem.UpdatedAt, uitem.ID); err != nil {
				return nil, nil, nil, err
			}
		}

		obtainItems = append(obtainItems, uitem)

	default:
		return nil, nil, nil, ErrInvalidItemType
	}

	return obtainCoins, obtainCards, obtainItems, nil
}
