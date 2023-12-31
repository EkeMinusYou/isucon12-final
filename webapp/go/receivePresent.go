package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
)

// receivePresent プレゼント受け取り
// POST /user/{userID}/present/receive
func (h *Handler) receivePresent(c echo.Context) error {
	defer c.Request().Body.Close()
	req := new(ReceivePresentRequest)
	if err := parseRequestBody(c, req); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	userID, err := getUserID(c)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	user := new(User)
	query := "SELECT * FROM users WHERE id=?"
	if err := h.DB.Get(user, query, userID); err != nil {
		if err == ErrUserNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	requestAt, err := getRequestTime(c)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, ErrGetRequestTime)
	}

	if len(req.PresentIDs) == 0 {
		return errorResponse(c, http.StatusUnprocessableEntity, fmt.Errorf("presentIds is empty"))
	}

	if err = h.checkViewerID(userID, req.ViewerID); err != nil {
		if err == ErrUserDeviceNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// 未取得のプレゼント取得
	query = "SELECT * FROM user_presents WHERE id IN (?) AND deleted_at IS NULL"
	query, params, err := sqlx.In(query, req.PresentIDs)
	if err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}
	obtainPresent := []*UserPresent{}
	if err = h.DB.Select(&obtainPresent, query, params...); err != nil {
		return errorResponse(c, http.StatusBadRequest, err)
	}

	if len(obtainPresent) == 0 {
		return successResponse(c, &ReceivePresentResponse{
			UpdatedResources: makeUpdatedResources(requestAt, nil, nil, nil, nil, nil, nil, []*UserPresent{}),
		})
	}

	// master check
	obtainCardPresent := make([]*UserPresent, 0)
	for _, v := range obtainPresent {
		if v.ItemType == 2 {
			obtainCardPresent = append(obtainCardPresent, v)
		}
	}

	obtainCardPresentItemIDs := make([]int64, 0)
	for _, v := range obtainCardPresent {
		obtainCardPresentItemIDs = append(obtainCardPresentItemIDs, v.ItemID)
	}

	cardItemMasters := make([]*ItemMaster, 0)
	if len(obtainCardPresentItemIDs) != 0 {
		query = "SELECT * FROM item_masters WHERE id IN (?) AND item_type = 2"
		query, params, err = sqlx.In(query, obtainCardPresentItemIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := h.DB.Select(&cardItemMasters, query, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	obtainEnhancePresent := make([]*UserPresent, 0)
	for _, v := range obtainPresent {
		if v.ItemType == 3 || v.ItemType == 4 {
			obtainEnhancePresent = append(obtainEnhancePresent, v)
		}
	}

	obtainEnhancePresentItemIDs := make([]int64, 0)
	for _, v := range obtainEnhancePresent {
		obtainEnhancePresentItemIDs = append(obtainEnhancePresentItemIDs, v.ItemID)
	}

	enhanceItemMasters := make([]*ItemMaster, 0)
	if len(obtainEnhancePresentItemIDs) != 0 {
		query = "SELECT * FROM item_masters WHERE id IN (?) AND item_type IN (3, 4)"
		query, params, err = sqlx.In(query, obtainEnhancePresentItemIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := h.DB.Select(&enhanceItemMasters, query, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// master check end

	tx, err := h.DB.Beginx()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	defer tx.Rollback() //nolint:errcheck

	obtainPresentIds := make([]int64, 0)
	for _, v := range obtainPresent {
		if v.ItemType != 1 && v.ItemType != 2 && v.ItemType != 3 && v.ItemType != 4 {
			return errorResponse(c, http.StatusBadRequest, ErrInvalidItemType)
		}
		obtainPresentIds = append(obtainPresentIds, v.ID)
	}

	query = "UPDATE user_presents SET deleted_at=?, updated_at=? WHERE id IN (?)"
	query, params, err = sqlx.In(query, requestAt, requestAt, obtainPresentIds)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	_, err = tx.Exec(query, params...)
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	for i := range obtainPresent {
		obtainPresent[i].UpdatedAt = requestAt
		obtainPresent[i].DeletedAt = &requestAt
	}

	obtainCoinPresent := make([]*UserPresent, 0)
	for _, v := range obtainPresent {
		if v.ItemType == 1 {
			obtainCoinPresent = append(obtainCoinPresent, v)
		}
	}

	// Coin配布処理
	amount := int64(0)
	for _, v := range obtainCoinPresent {
		amount += int64(v.Amount)
	}
	_, err = h.obtainCoinsForItemReceive(tx, user, amount)

	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// Card配布処理
	obtainCards := make([]*UserCard, 0)
	for _, v := range obtainCardPresent {
		var item *ItemMaster
		for _, itemMaster := range cardItemMasters {
			if v.ItemID == itemMaster.ID {
				item = itemMaster
				break
			}
		}
		if item == nil {
			return errorResponse(c, http.StatusNotFound, ErrItemNotFound)
		}

		cID, err := h.generateID()
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
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
		obtainCards = append(obtainCards, card)
	}
	// bulk insert
	if len(obtainCards) > 0 {
		_, err := tx.NamedExec("INSERT INTO user_cards(id, user_id, card_id, amount_per_sec, level, total_exp, created_at, updated_at) VALUES (:id, :user_id, :card_id, :amount_per_sec, :level, :total_exp, :created_at, :updated_at)", obtainCards)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// 強化アイテム配布処理
	uitems := make([]*UserItem, 0)
	if len(obtainEnhancePresentItemIDs) != 0 {
		query = "SELECT * FROM user_items WHERE user_id=? AND item_id IN (?)"
		query, params, err = sqlx.In(query, userID, obtainEnhancePresentItemIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := tx.Select(&uitems, query, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	upsertUitems := make([]*UserItem, 0)
	for _, v := range obtainEnhancePresent {
		var seen bool
		for _, itemMaster := range enhanceItemMasters {
			if v.ItemID == itemMaster.ID {
				seen = true
				break
			}
		}
		if !seen {
			return errorResponse(c, http.StatusNotFound, ErrItemNotFound)
		}

		var uitem *UserItem
		for _, candidate := range uitems {
			if v.ItemID == candidate.ItemID {
				uitem = candidate
				break
			}
		}

		if uitem == nil {
			uitemID, err := h.generateID()
			if err != nil {
				return errorResponse(c, http.StatusInternalServerError, err)
			}
			uitem := &UserItem{
				ID:        uitemID,
				UserID:    userID,
				ItemType:  v.ItemType,
				ItemID:    v.ItemID,
				Amount:    int(int64(v.Amount)),
				CreatedAt: requestAt,
				UpdatedAt: requestAt,
			}
			upsertUitems = append(upsertUitems, uitem)

		} else {
			uitem.Amount += int(int64(v.Amount))
			uitem.UpdatedAt = requestAt
			upsertUitems = append(upsertUitems, uitem)
		}
	}

	if len(upsertUitems) != 0 {
		query, err := BulkUpsertUserItems(tx, upsertUitems)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		_, err = tx.Exec(query)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	return successResponse(c, &ReceivePresentResponse{
		UpdatedResources: makeUpdatedResources(requestAt, nil, nil, nil, nil, nil, nil, obtainPresent),
	})
}

func BulkUpsertUserItems(db *sqlx.Tx, userItems []*UserItem) (string, error) {
	values := []string{}
	for _, item := range userItems {
		values = append(values, fmt.Sprintf("(%d, %d, %d, %d, %d, %d, %d)", item.ID, item.UserID, item.ItemID, item.ItemType, item.Amount, item.CreatedAt, item.UpdatedAt))
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
		"user_items",
		"id, user_id, item_id, item_type, amount, created_at, updated_at",
		strings.Join(values, ", "),
		"amount = VALUES(amount), updated_at = VALUES(updated_at)",
	)

	return query, nil
}
