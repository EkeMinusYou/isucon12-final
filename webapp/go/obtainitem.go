package main

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// obtainItem アイテム付与処理
func (h *Handler) obtainItem(tx *sqlx.Tx, userID, itemID int64, itemType int, obtainAmount int64, requestAt int64) ([]int64, []*UserCard, []*UserItem, error) {
	switch itemType {
	case 1: // coin
		obtainCoins, err := h.obtainCoins(tx, userID, obtainAmount)
		if err != nil {
			return nil, nil, nil, err
		}
		return obtainCoins, nil, nil, nil
	case 2: // card(ハンマー)
		obtainCards, err := h.obtainCards(tx, userID, itemID, itemType, requestAt)
		if err != nil {
			return nil, nil, nil, err
		}
		return nil, obtainCards, nil, nil
	case 3, 4: // 強化素材
		obtainItems, err := h.obtianEnhanceItem(tx, userID, itemID, itemType, obtainAmount, requestAt)
		if err != nil {
			return nil, nil, nil, err
		}
		return nil, nil, obtainItems, nil
	default:
		return nil, nil, nil, ErrInvalidItemType
	}
}

func (h *Handler) obtainCoins(tx *sqlx.Tx, userID int64, obtainAmount int64) ([]int64, error) {
	obtainCoins := make([]int64, 0)

	user := new(User)
	query := "SELECT * FROM users WHERE id=?"
	if err := tx.Get(user, query, userID); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}

	query = "UPDATE users SET isu_coin=? WHERE id=?"
	totalCoin := user.IsuCoin + obtainAmount
	if _, err := tx.Exec(query, totalCoin, user.ID); err != nil {
		return nil, err
	}
	obtainCoins = append(obtainCoins, obtainAmount)

	return obtainCoins, nil
}

func (h *Handler) obtainCards(tx *sqlx.Tx, userID, itemID int64, itemType int, requestAt int64) ([]*UserCard, error) {
	obtainCards := make([]*UserCard, 0)

	query := "SELECT * FROM item_masters WHERE id=? AND item_type=?"
	item := new(ItemMaster)
	if err := tx.Get(item, query, itemID, itemType); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrItemNotFound
		}
		return nil, err
	}

	cID, err := h.generateID()
	if err != nil {
		return nil, err
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
		return nil, err
	}
	obtainCards = append(obtainCards, card)

	return obtainCards, err
}

func (h *Handler) obtianEnhanceItem(tx *sqlx.Tx, userID, itemID int64, itemType int, obtainAmount int64, requestAt int64) ([]*UserItem, error) {
	obtainItems := make([]*UserItem, 0)

	query := "SELECT * FROM item_masters WHERE id=? AND item_type=?"
	item := new(ItemMaster)
	if err := tx.Get(item, query, itemID, itemType); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrItemNotFound
		}
		return nil, err
	}

	query = "SELECT * FROM user_items WHERE user_id=? AND item_id=?"
	uitem := new(UserItem)
	if err := tx.Get(uitem, query, userID, item.ID); err != nil {
		if err != sql.ErrNoRows {
			return nil, err
		}
		uitem = nil
	}

	if uitem == nil {
		uitemID, err := h.generateID()
		if err != nil {
			return nil, err
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
			return nil, err
		}

	} else {
		uitem.Amount += int(obtainAmount)
		uitem.UpdatedAt = requestAt
		query = "UPDATE user_items SET amount=?, updated_at=? WHERE id=?"
		if _, err := tx.Exec(query, uitem.Amount, uitem.UpdatedAt, uitem.ID); err != nil {
			return nil, err
		}
	}

	obtainItems = append(obtainItems, uitem)

	return obtainItems, nil
}
