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
	query := "SELECT * FROM user_presents WHERE id IN (?) AND deleted_at IS NULL"
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
	user := new(User)
	query = "SELECT * FROM users WHERE id=?"
	if err := tx.Get(user, query, userID); err != nil {
		if err == ErrUserNotFound {
			return errorResponse(c, http.StatusNotFound, err)
		}
		return errorResponse(c, http.StatusInternalServerError, err)
	}
	amount := int64(0)
	for _, v := range obtainCoinPresent {
		amount += int64(v.Amount)
	}
	_, err = h.obtainCoinsForItemReceive(tx, user, amount)

	if err != nil {
		return errorResponse(c, http.StatusInternalServerError, err)
	}

	// 配布処理
	for _, v := range obtainPresent {
		if v.ItemType != 2 {
			continue
		}

		_, err = h.obtainCards(tx, v.UserID, v.ItemID, v.ItemType, requestAt)

		if err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// 強化アイテム配布処理
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

	itemMasters := make([]*ItemMaster, 0)
	if len(obtainEnhancePresentItemIDs) != 0 {
		query = "SELECT * FROM item_masters WHERE id IN (?) AND item_type IN (3, 4)"
		query, params, err = sqlx.In(query, obtainEnhancePresentItemIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		if err := tx.Select(&itemMasters, query, params...); err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

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

	updateUitems := make([]*UserItem, 0)
	for _, v := range obtainEnhancePresent {
		var seen bool
		for _, itemMaster := range itemMasters {
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
			_, err = h.obtainEnhanceItemForRecieveItem(tx, userID, v.ItemID, v.ItemType, int64(v.Amount), requestAt)

			if err != nil {
				if err == ErrUserNotFound {
					return errorResponse(c, http.StatusNotFound, err)
				}
				return errorResponse(c, http.StatusInternalServerError, err)
			}
		} else {
			uitem.Amount += int(int64(v.Amount))
			uitem.UpdatedAt = requestAt
			updateUitems = append(updateUitems, uitem)
		}
	}

	if len(updateUitems) != 0 {
		query, err := BulkUpsertUserItems(tx, "user_items", updateUitems)
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

func BulkUpsertUserItems(db *sqlx.Tx, table string, userItems []*UserItem) (string, error) {
	values := []string{}
	for _, item := range userItems {
		values = append(values, fmt.Sprintf("(%d, %d, %d, %d, %d, %d, %d)", item.ID, item.UserID, item.ItemID, item.ItemType, item.Amount, item.CreatedAt, item.UpdatedAt))
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
		table,
		"id, user_id, item_id, item_type, amount, created_at, updated_at",
		strings.Join(values, ", "),
		"amount = VALUES(amount), updated_at = VALUES(updated_at)",
	)

	return query, nil
}
