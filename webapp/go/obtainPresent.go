package main

import "github.com/jmoiron/sqlx"

// obtainPresent プレゼント付与
func (h *Handler) obtainPresent(tx *sqlx.Tx, userID int64, requestAt int64) ([]*UserPresent, error) {
	normalPresents := make([]*PresentAllMaster, 0)
	query := `
SELECT
  pam.ID AS id,
  pam.registered_start_at AS registered_start_at,
  pam.registered_end_at AS registered_end_at,
  pam.item_type AS item_type,
  pam.item_id AS item_id,
  pam.amount AS amount,
  pam.present_message AS present_message,
  pam.created_at AS created_at
FROM
  present_all_masters pam
  LEFT JOIN user_present_all_received_history uparh ON pam.id = uparh.present_all_id AND uparh.user_id = ?
WHERE
  pam.registered_start_at <= ? AND
  pam.registered_end_at >= ? AND
  uparh.ID IS NULL
`
	if err := tx.Select(&normalPresents, query, userID, requestAt, requestAt); err != nil {
		return nil, err
	}

	obtainPresents := make([]*UserPresent, 0)

	upList := make([]UserPresent, 0)
	historyList := make([]UserPresentAllReceivedHistory, 0)

	for _, np := range normalPresents {
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
		historyList = append(historyList, *history)
	}

	// bulk insert
	if len(upList) > 0 {
		_, err := tx.NamedExec("INSERT INTO user_presents(id, user_id, sent_at, item_type, item_id, amount, present_message, created_at, updated_at) VALUES (:id, :user_id, :sent_at, :item_type, :item_id, :amount, :present_message, :created_at, :updated_at)", upList)
		if err != nil {
			return nil, err
		}
		_, err = tx.NamedExec("INSERT INTO user_present_all_received_history(id, user_id, present_all_id, received_at, created_at, updated_at) VALUES (:id, :user_id, :present_all_id, :received_at, :created_at, :updated_at)", historyList)
		if err != nil {
			return nil, err
		}
	}

	return obtainPresents, nil
}
