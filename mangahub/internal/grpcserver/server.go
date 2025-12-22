package grpcserver

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"mangahub/internal/library"
	"mangahub/internal/manga"
	"mangahub/pkg/grpc/mangapb"
	"mangahub/pkg/models"
)

type Server struct {
	mangapb.UnimplementedMangaServiceServer
	mangapb.UnimplementedProgressServiceServer
	MangaRepo   *manga.Repo
	LibraryRepo *library.Repo
}

func NewServer(mangaRepo *manga.Repo, libraryRepo *library.Repo) *Server {
	return &Server{MangaRepo: mangaRepo, LibraryRepo: libraryRepo}
}

func (s *Server) ListManga(ctx context.Context, req *mangapb.ListMangaRequest) (*mangapb.ListMangaResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}
	query := manga.ListQuery{
		Q:      strings.TrimSpace(req.GetQ()),
		Genres: req.GetGenres(),
		Status: strings.TrimSpace(req.GetStatus()),
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	}

	total, err := s.MangaRepo.Count(ctx, query)
	if err != nil {
		return nil, status.Error(codes.Internal, "count failed")
	}

	items, err := s.MangaRepo.List(ctx, query)
	if err != nil {
		return nil, status.Error(codes.Internal, "list failed")
	}

	resp := &mangapb.ListMangaResponse{
		Total:  int32(total),
		Limit:  int32(query.Limit),
		Offset: int32(query.Offset),
		Items:  make([]*mangapb.Manga, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, mangaToProto(item))
	}
	return resp, nil
}

func (s *Server) GetManga(ctx context.Context, req *mangapb.GetMangaRequest) (*mangapb.GetMangaResponse, error) {
	if req == nil || strings.TrimSpace(req.GetId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "id required")
	}

	item, err := s.MangaRepo.GetByID(ctx, strings.TrimSpace(req.GetId()))
	if err != nil {
		return nil, status.Error(codes.Internal, "get failed")
	}
	if item == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}

	return &mangapb.GetMangaResponse{Manga: mangaToProto(*item)}, nil
}

func (s *Server) ListProgress(ctx context.Context, req *mangapb.ListProgressRequest) (*mangapb.ListProgressResponse, error) {
	if req == nil || strings.TrimSpace(req.GetUserId()) == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id required")
	}

	statusFilter := normalizeStatus(req.GetStatus())
	if req.GetStatus() != "" && statusFilter == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid status filter")
	}

	items, total, err := s.LibraryRepo.List(
		ctx,
		strings.TrimSpace(req.GetUserId()),
		statusFilter,
		int(req.GetLimit()),
		int(req.GetOffset()),
	)
	if err != nil {
		return nil, status.Error(codes.Internal, "list failed")
	}

	resp := &mangapb.ListProgressResponse{
		Total:  int32(total),
		Limit:  int32(req.GetLimit()),
		Offset: int32(req.GetOffset()),
		Items:  make([]*mangapb.ProgressItem, 0, len(items)),
	}
	for _, item := range items {
		resp.Items = append(resp.Items, progressToProto(item))
	}
	return resp, nil
}

func (s *Server) GetProgress(ctx context.Context, req *mangapb.GetProgressRequest) (*mangapb.GetProgressResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	mangaID := strings.TrimSpace(req.GetMangaId())
	if userID == "" || mangaID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and manga_id required")
	}

	item, err := s.LibraryRepo.Get(ctx, userID, mangaID)
	if err != nil {
		return nil, status.Error(codes.Internal, "get failed")
	}
	if item == nil {
		return nil, status.Error(codes.NotFound, "not found")
	}

	return &mangapb.GetProgressResponse{Item: progressToProto(*item)}, nil
}

func (s *Server) UpsertProgress(ctx context.Context, req *mangapb.UpsertProgressRequest) (*mangapb.UpsertProgressResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request required")
	}
	userID := strings.TrimSpace(req.GetUserId())
	mangaID := strings.TrimSpace(req.GetMangaId())
	if userID == "" || mangaID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and manga_id required")
	}

	statusValue := normalizeStatus(req.GetStatus())
	if statusValue == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid status")
	}

	if req.GetCurrentChapter() < 0 {
		return nil, status.Error(codes.InvalidArgument, "current_chapter must be >= 0")
	}

	item := models.LibraryItem{
		UserID:         userID,
		MangaID:        mangaID,
		CurrentChapter: int(req.GetCurrentChapter()),
		Status:         statusValue,
	}

	if err := s.LibraryRepo.Upsert(ctx, item); err != nil {
		return nil, status.Error(codes.Internal, "save failed")
	}

	saved, err := s.LibraryRepo.Get(ctx, userID, mangaID)
	if err != nil {
		return nil, status.Error(codes.Internal, "fetch failed")
	}
	if saved == nil {
		return nil, status.Error(codes.Internal, "saved item not found")
	}

	return &mangapb.UpsertProgressResponse{Item: progressToProto(*saved)}, nil
}

func (s *Server) DeleteProgress(ctx context.Context, req *mangapb.DeleteProgressRequest) (*mangapb.DeleteProgressResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	mangaID := strings.TrimSpace(req.GetMangaId())
	if userID == "" || mangaID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and manga_id required")
	}

	deleted, err := s.LibraryRepo.Delete(ctx, userID, mangaID)
	if err != nil {
		return nil, status.Error(codes.Internal, "delete failed")
	}
	if !deleted {
		return nil, status.Error(codes.NotFound, "not found")
	}

	return &mangapb.DeleteProgressResponse{Deleted: true}, nil
}

func mangaToProto(item models.MangaDB) *mangapb.Manga {
	return &mangapb.Manga{
		Id:            item.ID,
		Title:         item.Title,
		Author:        item.Author,
		Genres:        item.Genres,
		Status:        item.Status,
		TotalChapters: int32(item.TotalChapters),
		Description:   item.Description,
		CoverUrl:      item.CoverURL,
	}
}

func progressToProto(item models.LibraryItem) *mangapb.ProgressItem {
	return &mangapb.ProgressItem{
		UserId:         item.UserID,
		MangaId:        item.MangaID,
		CurrentChapter: int32(item.CurrentChapter),
		Status:         item.Status,
		UpdatedAtUnix:  item.UpdatedAt.Unix(),
	}
}

func normalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "reading":
		return "reading"
	case "completed":
		return "completed"
	case "wish list", "wish_list", "wishlist":
		return "wish_list"
	case "blacklist", "black_list", "black list":
		return "blacklist"
	case "":
		return ""
	default:
		return ""
	}
}
