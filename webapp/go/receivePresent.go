package main

import (
	"fmt"
	"net/http"

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

	for _, v := range obtainPresent {
		if v.ItemType != 1 && v.ItemType != 2 && v.ItemType != 3 && v.ItemType != 4 {
			return errorResponse(c, http.StatusBadRequest, ErrInvalidItemType)
		}
	}

	for i := range obtainPresent {
		obtainPresent[i].UpdatedAt = requestAt
		obtainPresent[i].DeletedAt = &requestAt
	}

	// 配布処理
	for i := range obtainPresent {
		v := obtainPresent[i]

		if v.ItemType != 1 {
			continue
		}

		query = "UPDATE user_presents SET deleted_at=?, updated_at=? WHERE id=?"
		_, err := tx.Exec(query, requestAt, requestAt, v.ID)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		_, err = h.obtainCoins(tx, userID, int64(v.Amount))

		if err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// 配布処理
	for _, v := range obtainPresent {
		if v.ItemType != 2 {
			continue
		}

		query = "UPDATE user_presents SET deleted_at=?, updated_at=? WHERE id=?"
		_, err := tx.Exec(query, requestAt, requestAt, v.ID)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}

		_, err = h.obtainCards(tx, v.UserID, v.ItemID, v.ItemType, requestAt)

		if err != nil {
			if err == ErrUserNotFound || err == ErrItemNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
			return errorResponse(c, http.StatusInternalServerError, err)
		}
	}

	// 配布処理
	obtainEnhancePresent := make([]*UserPresent, 0)
	for _, v := range obtainPresent {
		if v.ItemType == 3 || v.ItemType == 4 {
			obtainEnhancePresent = append(obtainEnhancePresent, v)
		}
	}

	var enhancePresentIDs []int64
	for _, v := range obtainEnhancePresent {
		enhancePresentIDs = append(enhancePresentIDs, v.ID)
	}
	if len(obtainEnhancePresent) != 0 {
		query = "UPDATE user_presents SET deleted_at=?, updated_at=? WHERE id IN (?)"
		query, params, err = sqlx.In(query, requestAt, requestAt, enhancePresentIDs)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
		}
		_, err = tx.Exec(query, params...)
		if err != nil {
			return errorResponse(c, http.StatusInternalServerError, err)
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

		_, err = h.obtianEnhanceItemForRecieveItem(tx, userID, uitem, v.ItemID, v.ItemType, int64(v.Amount), requestAt)

		if err != nil {
			if err == ErrUserNotFound {
				return errorResponse(c, http.StatusNotFound, err)
			}
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
