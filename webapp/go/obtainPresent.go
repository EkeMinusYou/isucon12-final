package main

import "github.com/jmoiron/sqlx"

// obtainPresent プレゼント付与
func (h *Handler) obtainPresent(tx *sqlx.Tx, userID int64, requestAt int64) ([]*UserPresent, error) {
	normalPresents := make([]*PresentAllMaster, 0)
	query := "SELECT * FROM present_all_masters WHERE registered_start_at <= ? AND registered_end_at >= ?"
	if err := tx.Select(&normalPresents, query, requestAt, requestAt); err != nil {
		return nil, err
	}

	obtainPresents := make([]*UserPresent, 0)

	receivedList := make([]*UserPresentAllReceivedHistory, 0)
	var presentIds []int64
	for _, np := range normalPresents {
		presentIds = append(presentIds, np.ID)
	}
	query, args, err := sqlx.In("SELECT * FROM user_present_all_received_history WHERE user_id=? AND present_all_id IN (?)", userID, presentIds)
	if err != nil {
		return nil, err
	}
	err = tx.Select(&receivedList, query, args...)
	if err != nil {
		return nil, err
	}

	upList := make([]UserPresent, 0)
L1:
	for _, np := range normalPresents {
		// プレゼント配布済
		for _, received := range receivedList {
			if received.PresentAllID == np.ID {
				continue L1
			}
		}

		pID, err := h.generateID()
		if err != nil {
			return nil, err
		}
		up := &UserPresent{
			ID:             pID,
			UserID:         userID,
			SentAt:         requestAt,
			ItemType:       np.ItemType,
			ItemID:         np.ItemID,
			Amount:         int(np.Amount),
			PresentMessage: np.PresentMessage,
			CreatedAt:      requestAt,
			UpdatedAt:      requestAt,
		}
		upList = append(upList, *up)

		obtainPresents = append(obtainPresents, up)
	}

	// bulk insert
	if len(upList) > 0 {
		_, err := tx.NamedExec("INSERT INTO user_presents(id, user_id, sent_at, item_type, item_id, amount, present_message, created_at, updated_at) VALUES (:id, :user_id, :sent_at, :item_type, :item_id, :amount, :present_message, :created_at, :updated_at)", upList)
		if err != nil {
			return nil, err
		}
	}

L2:
	for _, np := range normalPresents {
		// プレゼント配布済
		for _, received := range receivedList {
			if received.PresentAllID == np.ID {
				continue L2
			}
		}

		phID, err := h.generateID()
		if err != nil {
			return nil, err
		}
		history := &UserPresentAllReceivedHistory{
			ID:           phID,
			UserID:       userID,
			PresentAllID: np.ID,
			ReceivedAt:   requestAt,
			CreatedAt:    requestAt,
			UpdatedAt:    requestAt,
		}
		query = "INSERT INTO user_present_all_received_history(id, user_id, present_all_id, received_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)"
		if _, err := tx.Exec(
			query,
			history.ID,
			history.UserID,
			history.PresentAllID,
			history.ReceivedAt,
			history.CreatedAt,
			history.UpdatedAt,
		); err != nil {
			return nil, err
		}
	}
	return obtainPresents, nil
}
