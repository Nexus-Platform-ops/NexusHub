package api

import (
	"errors"
	"strconv"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

// Helper functions for dynamic query building
func itoa(i int) string { return strconv.Itoa(i) }
func parseInt64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

type Product struct {
	ID          int64     `json:"id"`
	SellerID    int64     `json:"seller_id"`
	Seller      string    `json:"seller"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	PriceCoins  int64     `json:"price_coins"`
	PriceMMK    int64     `json:"price_mmk"`
	Township    string    `json:"township"`
	Category    string    `json:"category"`
	ImageURL    string    `json:"image_url"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"created_at"`
}

const productFields = `p.id, p.seller_id, u.display_name, p.title, p.description,
 p.price_coins, p.price_mmk, p.township, p.category, p.image_url, p.active, p.created_at`

const productFrom = ` FROM market.products p JOIN social.users u ON u.id = p.seller_id `

// handleListProducts returns active listings with search and filters.
func (s *Server) handleListProducts(w http.ResponseWriter, r *http.Request) {
	township := trimLimit(r.URL.Query().Get("township"), 80)
	category := trimLimit(r.URL.Query().Get("category"), 40)
	search := trimLimit(r.URL.Query().Get("search"), 100)
	priceMin := r.URL.Query().Get("price_min")
	priceMax := r.URL.Query().Get("price_max")
	sortBy := r.URL.Query().Get("sort") // "newest", "price_asc", "price_desc"

	// Build dynamic query
	where := " WHERE p.active"
	args := []any{}
	argIdx := 1

	if township != "" {
		where += ` AND p.township = $` + itoa(argIdx)
		args = append(args, township)
		argIdx++
	}
	if category != "" {
		where += ` AND p.category = $` + itoa(argIdx)
		args = append(args, category)
		argIdx++
	}
	if search != "" {
		where += ` AND p.search_vector @@ plainto_tsquery('english', $` + itoa(argIdx) + `)`
		args = append(args, search)
		argIdx++
	}
	if priceMin != "" {
		where += ` AND (p.price_coins >= $` + itoa(argIdx) + ` OR p.price_mmk >= $` + itoa(argIdx) + `)`
		args = append(args, parseInt64(priceMin))
		argIdx++
	}
	if priceMax != "" {
		where += ` AND (p.price_coins <= $` + itoa(argIdx) + ` OR p.price_mmk <= $` + itoa(argIdx) + `)`
		args = append(args, parseInt64(priceMax))
		argIdx++
	}

	// Sort order
	orderBy := " ORDER BY p.id DESC"
	switch sortBy {
	case "price_asc":
		orderBy = " ORDER BY COALESCE(NULLIF(p.price_coins, 0), p.price_mmk) ASC"
	case "price_desc":
		orderBy = " ORDER BY COALESCE(NULLIF(p.price_coins, 0), p.price_mmk) DESC"
	}

	query := `SELECT ` + productFields + productFrom + where + orderBy + ` LIMIT 50`
	rows, err := s.Store.Social.Query(r.Context(), query, args...)
	if err != nil {
		serverError(w, "list products", err)
		return
	}
	defer rows.Close()
	products := []Product{}
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.SellerID, &p.Seller, &p.Title, &p.Description,
			&p.PriceCoins, &p.PriceMMK, &p.Township, &p.Category, &p.ImageURL, &p.Active, &p.CreatedAt); err != nil {
			serverError(w, "scan product", err)
			return
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate products", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products})
}

// handleCreateProduct lists a new item (seller = caller).
func (s *Server) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		PriceCoins  int64  `json:"price_coins"`
		PriceMMK    int64  `json:"price_mmk"`
		Township    string `json:"township"`
		Category    string `json:"category"`
		ImageURL    string `json:"image_url"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.Title = trimSpace(req.Title)
	if len(req.Title) < 2 || len(req.Title) > 120 {
		errorJSON(w, http.StatusBadRequest, "title must be 2-120 characters")
		return
	}
	req.Description = trimLimit(req.Description, 500)
	req.Township = trimLimit(req.Township, 80)
	req.Category = trimLimit(req.Category, 40)
	if req.Category == "" {
		req.Category = "other"
	}
	req.ImageURL = trimLimit(req.ImageURL, 500)
	if req.PriceCoins < 0 || req.PriceMMK < 0 {
		errorJSON(w, http.StatusBadRequest, "prices must be non-negative")
		return
	}
	if req.PriceCoins == 0 && req.PriceMMK == 0 {
		errorJSON(w, http.StatusBadRequest, "at least one price (coins or MMK) is required")
		return
	}
	var id int64
	var createdAt time.Time
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO market.products
		   (seller_id, title, description, price_coins, price_mmk, township, category, image_url, active)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,true)
		 RETURNING id, created_at`,
		uid, req.Title, req.Description, req.PriceCoins, req.PriceMMK, req.Township, req.Category, req.ImageURL,
	).Scan(&id, &createdAt); err != nil {
		serverError(w, "create product", err)
		return
	}
	writeJSON(w, http.StatusCreated, Product{
		ID: id, SellerID: uid, Title: req.Title, Description: req.Description,
		PriceCoins: req.PriceCoins, PriceMMK: req.PriceMMK, Township: req.Township,
		Category: req.Category, ImageURL: req.ImageURL, Active: true, CreatedAt: createdAt,
	})
}

// handleCreateOrder buys a listing with Nexus Coins atomically: the buyer's
// balance is checked under an advisory lock, the ledger records both legs
// and the listing is deactivated inside one transaction.
func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		ProductID int64 `json:"product_id"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.ProductID <= 0 {
		errorJSON(w, http.StatusBadRequest, "product_id is required")
		return
	}
	ctx := r.Context()
	tx, err := s.Store.Social.Begin(ctx)
	if err != nil {
		serverError(w, "begin order tx", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, uid); err != nil {
		serverError(w, "lock buyer", err)
		return
	}
	var sellerID, priceCoins int64
	var active bool
	err = tx.QueryRow(ctx,
		`SELECT seller_id, price_coins, active FROM market.products WHERE id=$1 FOR UPDATE`,
		req.ProductID).Scan(&sellerID, &priceCoins, &active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "product not found")
		} else {
			serverError(w, "load product", err)
		}
		return
	}
	if !active {
		errorJSON(w, http.StatusConflict, "product no longer available")
		return
	}
	if sellerID == uid {
		errorJSON(w, http.StatusBadRequest, "cannot buy your own listing")
		return
	}
	if priceCoins == 0 {
		errorJSON(w, http.StatusBadRequest,
			"this listing is priced in MMK; coin purchase is not available")
		return
	}
	var balance int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount),0) FROM ledger.coin_transactions WHERE user_id=$1`,
		uid).Scan(&balance); err != nil {
		serverError(w, "read balance", err)
		return
	}
	if balance < priceCoins {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "insufficient coins", "balance": balance, "price": priceCoins,
		})
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger.coin_transactions (user_id, amount, kind) VALUES ($1,$2,'purchase')`,
		uid, -priceCoins); err != nil {
		serverError(w, "debit buyer", err)
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger.coin_transactions (user_id, amount, kind) VALUES ($1,$2,'sale')`,
		sellerID, priceCoins); err != nil {
		serverError(w, "credit seller", err)
		return
	}
	var orderID int64
	var createdAt time.Time
	if err := tx.QueryRow(ctx,
		`INSERT INTO market.orders (product_id, buyer_id, price_coins, status)
		 VALUES ($1,$2,$3,'paid') RETURNING id, created_at`,
		req.ProductID, uid, priceCoins).Scan(&orderID, &createdAt); err != nil {
		serverError(w, "insert order", err)
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE market.products SET active=false WHERE id=$1`, req.ProductID); err != nil {
		serverError(w, "deactivate product", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "commit order", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"order": map[string]any{
			"id": orderID, "product_id": req.ProductID, "buyer_id": uid,
			"price_coins": priceCoins, "status": "paid", "created_at": createdAt,
		},
		"balance": balance - priceCoins,
	})
}

// handleGetSellerInfo returns seller rating and verification status.
func (s *Server) handleGetSellerInfo(w http.ResponseWriter, r *http.Request) {
	sellerIDStr := r.PathValue("id")
	sellerID, err := strconv.ParseInt(sellerIDStr, 10, 64)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid seller id")
		return
	}

	var phoneVerified bool
	var completedSales int64
	var avgRating float64
	var ratingCount int64

	err = s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(phone_verified, false),
		        COALESCE((SELECT COUNT(*) FROM market.orders WHERE seller_id = $1 AND status = 'paid'), 0),
		        COALESCE((SELECT AVG(rating) FROM market.seller_ratings WHERE seller_id = $1), 0),
		        COALESCE((SELECT COUNT(*) FROM market.seller_ratings WHERE seller_id = $1), 0)
		 FROM social.users WHERE id = $1`,
		sellerID).Scan(&phoneVerified, &completedSales, &avgRating, &ratingCount)
	if err != nil {
		serverError(w, "get seller info", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"seller_id":       sellerID,
		"phone_verified":  phoneVerified,
		"completed_sales": completedSales,
		"avg_rating":      avgRating,
		"rating_count":    ratingCount,
		"verified_seller": phoneVerified && completedSales >= 3,
	})
}

// handleCreateSellerRating allows a buyer to rate a seller after purchase.
func (s *Server) handleCreateSellerRating(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		SellerID  int64  `json:"seller_id"`
		ProductID int64  `json:"product_id"`
		Rating    int    `json:"rating"`
		Comment   string `json:"comment"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		errorJSON(w, http.StatusBadRequest, "rating must be 1-5")
		return
	}
	req.Comment = trimLimit(req.Comment, 500)

	var id int64
	var createdAt time.Time
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO market.seller_ratings (seller_id, buyer_id, product_id, rating, comment)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		req.SellerID, uid, req.ProductID, req.Rating, req.Comment,
	).Scan(&id, &createdAt); err != nil {
		serverError(w, "create rating", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         id,
		"rating":     req.Rating,
		"comment":    req.Comment,
		"created_at": createdAt,
	})
}
