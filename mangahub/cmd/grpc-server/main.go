package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	"mangahub/internal/grpcserver"
	"mangahub/internal/library"
	"mangahub/internal/manga"
	"mangahub/pkg/database"
	"mangahub/pkg/grpc/mangapb"
	"mangahub/pkg/utils"
)

func main() {
	cfg := database.DefaultConfig()
	db := database.MustOpen(cfg)
	defer db.Close()

	if err := database.Migrate(db); err != nil {
		log.Fatalf("db migrate failed: %v", err)
	}

	grpcCfg := utils.LoadGrpcConfig()
	listener, err := net.Listen("tcp", grpcCfg.Addr)
	if err != nil {
		log.Fatalf("grpc listen failed: %v", err)
	}

	mangaRepo := manga.NewRepo(db)
	libraryRepo := library.NewRepo(db)
	svc := grpcserver.NewServer(mangaRepo, libraryRepo)

	grpcServer := grpc.NewServer()
	mangapb.RegisterMangaServiceServer(grpcServer, svc)
	mangapb.RegisterProgressServiceServer(grpcServer, svc)

	log.Printf("gRPC server listening on %s", grpcCfg.Addr)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("grpc server stopped: %v", err)
	}
}
