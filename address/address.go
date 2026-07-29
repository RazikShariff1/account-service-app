package address

import "time"

// AddressRequest is the payload accepted by the create and update endpoints.
type AddressRequest struct {
	RoadId    int      `json:"road_id"`
	DoorNo    *string  `json:"door_no"`
	Landmark  *string  `json:"landmark"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Pincode   *string  `json:"pincode"`
	Country   string   `json:"country"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Img       *string  `json:"img"`
}

// AddressResponse is the view returned by the get/list endpoints.
type AddressResponse struct {
	Id        int       `json:"id"`
	DoorNo    *string   `json:"door_no"`
	Landmark  *string   `json:"landmark"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	Pincode   *string   `json:"pincode"`
	Country   string    `json:"country"`
	Latitude  *float64  `json:"latitude"`
	Longitude *float64  `json:"longitude"`
	Img       *string   `json:"img"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
