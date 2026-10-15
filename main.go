package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	_ "github.com/mattn/go-sqlite3"
	"github.com/shirou/gopsutil/v3/process"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go tracker tracker.c

// AppMeta هيكل تخزين بيانات البرامج الرسمية (الاسم والأيقونة)
type AppMeta struct {
	Name string
	Icon string
}

// ResolvedProcess هيكل لحفظ بيانات العملية التي تم التعرف عليها حتى لا تضيع عند إغلاق البرنامج
type ResolvedProcess struct {
	Name     string
	FullPath string
	Icon     string
}

// قاموس (Cache) يخزن كل البرامج الموجودة على النظام في الذاكرة
var desktopCache map[string]AppMeta

// ذاكرة مؤقتة لربط الـ PID بهويته الحقيقية طوال فترة التشغيل وحتى بعد إغلاقه
var pidMetaCache = make(map[uint32]ResolvedProcess)

// buildDesktopCache دالة لمسح وقراءة كل ملفات .desktop في النظام
func buildDesktopCache() {
	desktopCache = make(map[string]AppMeta)

	dirs := []string{
		"/usr/share/applications",
		"/var/lib/flatpak/exports/share/applications",
	}

	homeDirs, _ := filepath.Glob("/home/*/.local/share/applications")
	dirs = append(dirs, homeDirs...)
	dirs = append(dirs, "/root/.local/share/applications")

	for _, dir := range dirs {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			parseDesktopFile(path)
			return nil
		})
	}
}

// parseDesktopFile دالة استخراج البيانات من ملف .desktop الواحد
func parseDesktopFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	var name, icon, execCmd string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Name=") && name == "" {
			name = strings.TrimPrefix(line, "Name=")
		} else if strings.HasPrefix(line, "Icon=") && icon == "" {
			icon = strings.TrimPrefix(line, "Icon=")
		} else if strings.HasPrefix(line, "Exec=") && execCmd == "" {
			execCmd = strings.TrimPrefix(line, "Exec=")
		}
	}

	if name == "" {
		return
	}

	appID := strings.TrimSuffix(filepath.Base(path), ".desktop")
	desktopCache[strings.ToLower(appID)] = AppMeta{Name: name, Icon: icon}

	if execCmd != "" {
		execParts := strings.Fields(execCmd)
		if len(execParts) > 0 {
			binName := filepath.Base(execParts[0])
			desktopCache[strings.ToLower(binName)] = AppMeta{Name: name, Icon: icon}
		}
	}
}

// getCurrentNetworkInfo جلب اسم الشبكة الحالية
func getCurrentNetworkInfo() string {
	cmd := exec.Command("sh", "-c", "nmcli -t -f BSSID,ACTIVE,SSID dev wifi | grep ':yes:' | head -n 1")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		line := strings.TrimSpace(string(out))
		line = strings.ReplaceAll(line, `\:`, `-`)
		parts := strings.Split(line, ":yes:")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return "Unknown_Network"
}

// matchFromCache دالة مساعدة للبحث في القاموس مع دعم الأسماء المقصوصة عند 15 حرفاً
func matchFromCache(id string) (AppMeta, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return AppMeta{}, false
	}
	if meta, ok := desktopCache[id]; ok {
		return meta, true
	}
	if len(id) >= 15 {
		for key, meta := range desktopCache {
			if strings.HasPrefix(key, id) {
				return meta, true
			}
		}
	}
	return AppMeta{}, false
}

// resolveAppMeta الدالة الرئيسية والموحدة لترجمة العملية إلى هوية رسومية
func resolveAppMeta(pid uint32, kernelComm string) (string, string, string) {
	defaultIcon := "application-x-executable"

	// 0. إذا كنا قد تعرفنا على هذا الـ PID من قبل أثناء عمله، نرجع بياناته المحفوظة فوراً
	if cached, ok := pidMetaCache[pid]; ok && cached.Icon != defaultIcon {
		return cached.Name, cached.FullPath, cached.Icon
	}

	cleanComm := strings.TrimSpace(kernelComm)
	lowerComm := strings.ToLower(cleanComm)

	saveAndReturn := func(name, fullPath, icon string) (string, string, string) {
		if icon != defaultIcon {
			pidMetaCache[pid] = ResolvedProcess{Name: name, FullPath: fullPath, Icon: icon}
		}
		return name, fullPath, icon
	}

	// 1. فحص برامج Flatpak
	flatpakInfoPath := fmt.Sprintf("/proc/%d/root/.flatpak-info", pid)
	if infoBytes, err := os.ReadFile(flatpakInfoPath); err == nil {
		for _, line := range strings.Split(string(infoBytes), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "name=") {
				appID := strings.TrimPrefix(line, "name=")
				if meta, ok := matchFromCache(appID); ok {
					return saveAndReturn(meta.Name, "Flatpak: "+appID, meta.Icon)
				}
			}
		}
	}

	// 2. استخدام Cgroups بعد تنظيف الأسطر لدعم .scope و خدمات dbus في GNOME
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	if cgroupBytes, err := os.ReadFile(cgroupPath); err == nil {
		for _, line := range strings.Split(string(cgroupBytes), "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "app.slice") || strings.Contains(line, "app-") || strings.Contains(line, "dbus-") {
				parts := strings.Split(line, "/")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if (strings.HasPrefix(part, "app-") || strings.HasPrefix(part, "dbus-")) &&
						(strings.HasSuffix(part, ".scope") || strings.HasSuffix(part, ".service")) {

						appIdPart := part
						appIdPart = strings.TrimSuffix(appIdPart, ".scope")
						appIdPart = strings.TrimSuffix(appIdPart, ".service")

						prefixes := []string{"app-gnome-", "app-flatpak-", "app-kde-", "app-"}
						for _, p := range prefixes {
							if strings.HasPrefix(appIdPart, p) {
								appIdPart = strings.TrimPrefix(appIdPart, p)
								break
							}
						}

						if strings.HasPrefix(appIdPart, "dbus-") {
							if idx := strings.Index(appIdPart, "-org."); idx != -1 {
								appIdPart = appIdPart[idx+1:]
							} else if idx := strings.Index(appIdPart, "-com."); idx != -1 {
								appIdPart = appIdPart[idx+1:]
							}
						}

						if atIdx := strings.LastIndex(appIdPart, "@"); atIdx != -1 {
							appIdPart = appIdPart[:atIdx]
						} else if lastDash := strings.LastIndex(appIdPart, "-"); lastDash != -1 {
							appIdPart = appIdPart[:lastDash]
						}

						appIdPart = strings.ReplaceAll(appIdPart, "\\x2d", "-")
						lowerAppID := strings.ToLower(appIdPart)

						if meta, ok := matchFromCache(lowerAppID); ok {
							return saveAndReturn(meta.Name, "Systemd CGroup", meta.Icon)
						}
						for key, meta := range desktopCache {
							if key == lowerAppID || strings.HasPrefix(key, lowerAppID+".") {
								return saveAndReturn(meta.Name, "Systemd CGroup", meta.Icon)
							}
						}
					}
				}
			}
		}
	}

	// 3. استخدام gopsutil لجلب بيانات العملية ومسارها
	pidInt32 := int32(pid)
	p, err := process.NewProcess(pidInt32)
	if err != nil {
		if meta, ok := matchFromCache(lowerComm); ok {
			return saveAndReturn(meta.Name, "Unknown (Ended)", meta.Icon)
		}
		if cached, ok := pidMetaCache[pid]; ok {
			return cached.Name, cached.FullPath, cached.Icon
		}
		return cleanComm, "Unknown", defaultIcon
	}

	exePath, err := p.Exe()
	if err != nil || exePath == "" {
		exePath = "Unknown Path"
	}

	procName, err := p.Name()
	if err != nil {
		procName = cleanComm
	}

	// 4. دعم خاص للبرامج المحمولة الشهيرة مثل Tor Browser
	if strings.Contains(strings.ToLower(exePath), "tor-browser") {
		idx := strings.Index(strings.ToLower(exePath), "tor-browser")
		baseDir := exePath[:idx+len("tor-browser")]
		torIcon := filepath.Join(baseDir, "Browser/browser/chrome/icons/default/default128.png")
		if _, err := os.Stat(torIcon); err == nil {
			return saveAndReturn("Tor Browser", exePath, torIcon)
		}
		return saveAndReturn("Tor Browser", exePath, "web-browser")
	}

	// 5. فحص سطر الأوامر (Cmdline) للبرامج التي تعمل عبر محركات مثل gjs-console أو python
	if cmdSlice, err := p.CmdlineSlice(); err == nil && len(cmdSlice) > 1 {
		for _, arg := range cmdSlice[1:] {
			argBase := filepath.Base(arg)
			if meta, ok := matchFromCache(argBase); ok {
				return saveAndReturn(meta.Name, exePath, meta.Icon)
			}
		}
	}

	// 6. المطابقة النهائية باسم الملف التنفيذي أو اسم العملية أو الاسم المقصوص
	exeBaseName := strings.ToLower(filepath.Base(exePath))
	if meta, ok := matchFromCache(exeBaseName); ok {
		return saveAndReturn(meta.Name, exePath, meta.Icon)
	}

	if meta, ok := matchFromCache(procName); ok {
		return saveAndReturn(meta.Name, exePath, meta.Icon)
	}

	if meta, ok := matchFromCache(lowerComm); ok {
		return saveAndReturn(meta.Name, exePath, meta.Icon)
	}

	return procName, exePath, defaultIcon
}

func initDB() *sql.DB {
	db, err := sql.Open("sqlite3", "/opt/BosyCat/bosycat_v3.db")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS network_metrics (
		date_bucket DATE,
		ssid TEXT,
		pid INTEGER,
		app_name TEXT,
		full_path TEXT,
		icon_name TEXT,
		tx_bytes INTEGER,
		rx_bytes INTEGER,
		PRIMARY KEY (date_bucket, ssid, pid)
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}
	return db
}

func main() {
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatalf("Failed to remove memory limits: %v", err)
	}

	buildDesktopCache()
	log.Printf("Loaded Desktop Entries Cache. Found %d applications.", len(desktopCache))

	db := initDB()
	defer db.Close()
	log.Println("SQLite database initialized successfully.")

	var objs trackerObjects
	if err := loadTrackerObjects(&objs, nil); err != nil {
		log.Fatalf("Failed to load eBPF objects: %v", err)
	}
	defer objs.Close()

	kpTCPSend, _ := link.Kprobe("tcp_sendmsg", objs.TraceTcpSendmsg, nil)
	defer kpTCPSend.Close()
	kpTCPRecv, _ := link.Kretprobe("tcp_recvmsg", objs.TraceTcpRecvmsgExit, nil)
	defer kpTCPRecv.Close()

	kpUDPSend, _ := link.Kprobe("udp_sendmsg", objs.TraceUdpSendmsg, nil)
	defer kpUDPSend.Close()
	kpUDPRecv, _ := link.Kretprobe("udp_recvmsg", objs.TraceUdpRecvmsgExit, nil)
	defer kpUDPRecv.Close()

	log.Println("BosyCat: eBPF code injected! Capturing and bucketing data...")

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	type procStats struct {
		Tx uint64
		Rx uint64
	}
	lastStats := make(map[uint32]procStats)

	go func() {
		for range ticker.C {
			currentSSID := getCurrentNetworkInfo()
			dateBucket := time.Now().Format("2006-01-02")

			var key uint32
			var value struct {
				TxBytes uint64
				RxBytes uint64
				Comm    [16]byte
			}

			tx_db, err := db.Begin()
			if err != nil {
				continue
			}

			// عدم استبدال الاسم والأيقونة المسجلين إذا كانت القيمة الجديدة مجرد أيقونة افتراضية بعد إغلاق البرنامج
			stmt, err := tx_db.Prepare(`
				INSERT INTO network_metrics (date_bucket, ssid, pid, app_name, full_path, icon_name, tx_bytes, rx_bytes)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(date_bucket, ssid, pid) DO UPDATE SET
				app_name = CASE WHEN excluded.icon_name != 'application-x-executable' THEN excluded.app_name ELSE network_metrics.app_name END,
				full_path = CASE WHEN excluded.icon_name != 'application-x-executable' THEN excluded.full_path ELSE network_metrics.full_path END,
				icon_name = CASE WHEN excluded.icon_name != 'application-x-executable' THEN excluded.icon_name ELSE network_metrics.icon_name END,
				tx_bytes = network_metrics.tx_bytes + excluded.tx_bytes,
				rx_bytes = network_metrics.rx_bytes + excluded.rx_bytes;
			`)
			if err != nil {
				tx_db.Rollback()
				continue
			}

			iter := objs.TrackerMap.Iterate()
			for iter.Next(&key, &value) {
				prev, exists := lastStats[key]
				var deltaTx, deltaRx uint64

				if exists && value.TxBytes >= prev.Tx && value.RxBytes >= prev.Rx {
					deltaTx = value.TxBytes - prev.Tx
					deltaRx = value.RxBytes - prev.Rx
				} else {
					deltaTx = value.TxBytes
					deltaRx = value.RxBytes
				}

				lastStats[key] = procStats{Tx: value.TxBytes, Rx: value.RxBytes}

				if deltaTx == 0 && deltaRx == 0 {
					continue
				}

				if currentSSID == "Unknown_Network" {
					continue
				}

				kernelComm := string(bytes.TrimRight(value.Comm[:], "\x00"))

				appName, fullPath, iconName := resolveAppMeta(key, kernelComm)

				stmt.Exec(dateBucket, currentSSID, key, appName, fullPath, iconName, deltaTx, deltaRx)
			}

			stmt.Close()
			tx_db.Commit()
		}
	}()

	stopper := make(chan os.Signal, 1)
	signal.Notify(stopper, os.Interrupt, syscall.SIGTERM)
	<-stopper
	log.Println("\nStopping BosyCat and cleaning up...")
}