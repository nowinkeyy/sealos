package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/labring/image-cri-shim/pkg/shim"
    "github.com/labring/image-cri-shim/pkg/types"
    "github.com/labring/sealos/pkg/utils/logger"
)

func main() {
    types.SyncConfigFromConfigMap(context.Background(), types.DefaultImageCRIShimConfig)
    cfg, err := types.Unmarshal(types.DefaultImageCRIShimConfig)
    if err != nil {
        logger.Error("failed to read config: %v", err)
        os.Exit(1)
    }
    auth, err := cfg.PreProcess()
    if err != nil {
        logger.Error("failed to preprocess config: %v", err)
        os.Exit(1)
    }
    s, err := shim.NewShim(cfg, auth)
    if err != nil {
        logger.Error("failed to init shim: %v", err)
        os.Exit(1)
    }
    if err := s.Setup(); err != nil {
        logger.Error("setup failed: %v", err)
        os.Exit(1)
    }
    if err := s.Start(); err != nil {
        logger.Error("start failed: %v", err)
        os.Exit(1)
    }

    reload := cfg.ReloadInterval.Duration
    if reload <= 0 {
        reload = types.DefaultReloadInterval
    }
    go func() {
        ticker := time.NewTicker(reload)
        defer ticker.Stop()
        for range ticker.C {
            types.SyncConfigFromConfigMap(context.Background(), types.DefaultImageCRIShimConfig)
            updatedCfg, err := types.Unmarshal(types.DefaultImageCRIShimConfig)
            if err != nil {
                logger.Debug("skip reload: failed to read config: %v", err)
                continue
            }
            updatedAuth, err := updatedCfg.PreProcess()
            if err != nil {
                logger.Debug("skip reload: failed to preprocess config: %v", err)
                continue
            }
            s.UpdateAuth(updatedAuth)
            s.UpdateCache(shim.CacheOptionsFromConfig(updatedCfg))
            offlineDomains := make([]string, 0, len(updatedAuth.OfflineCRIConfigs))
            for d := range updatedAuth.OfflineCRIConfigs {
                offlineDomains = append(offlineDomains, d)
            }
            privateDomains := make([]string, 0, len(updatedAuth.CRIConfigs))
            for d := range updatedAuth.CRIConfigs {
                privateDomains = append(privateDomains, d)
            }
            logger.Info("configmap updated: offline=%v private=%v cache(size=%d, imageTTL=%v, domainTTL=%v)", offlineDomains, privateDomains, updatedCfg.Cache.ImageCacheSize, updatedCfg.Cache.ImageCacheTTL, updatedCfg.Cache.DomainCacheTTL)
        }
    }()

    ch := make(chan os.Signal, 1)
    signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
    <-ch
    s.Stop()
}