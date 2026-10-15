import sys
import os
import sqlite3
from PyQt6.QtWidgets import (QApplication, QMainWindow, QWidget, QVBoxLayout,
                             QHBoxLayout, QLabel, QComboBox, QPushButton,
                             QTableWidget, QTableWidgetItem, QHeaderView,
                             QDialog, QCalendarWidget)
from PyQt6.QtCore import QDate, Qt, QTimer, QSize
from PyQt6.QtGui import QIcon, QFont, QStandardItemModel, QStandardItem, QBrush, QColor

DB_PATH = "/opt/BosyCat/bosycat_v3.db"

def format_size(bytes_size):
    if bytes_size < 1024:
        return f"{bytes_size} B"
    elif bytes_size < 1024 * 1024:
        return f"{bytes_size / 1024:.2f} KB"
    elif bytes_size < 1024 * 1024 * 1024:
        return f"{bytes_size / (1024 * 1024):.2f} MB"
    else:
        return f"{bytes_size / (1024 ** 3):.2f} GB"

class DatePickerButton(QPushButton):
    def __init__(self, default_date=None):
        super().__init__()
        self.setObjectName("datePicker")
        self.current_date = default_date if default_date else QDate.currentDate()
        self.update_text()
        self.setCursor(Qt.CursorShape.PointingHandCursor)
        self.clicked.connect(self.show_calendar)

    def update_text(self):
        self.setText(self.current_date.toString('yyyy-MM-dd'))

    def date(self):
        return self.current_date

    def show_calendar(self):
        dialog = QDialog(self)
        dialog.setWindowFlags(Qt.WindowType.Popup | Qt.WindowType.FramelessWindowHint)
        layout = QVBoxLayout(dialog)
        layout.setContentsMargins(0, 0, 0, 0)
        
        cal = QCalendarWidget()
        cal.setGridVisible(False)
        cal.setSelectedDate(self.current_date)
        cal.setStyleSheet("""
            QCalendarWidget { background-color: white; border: 1px solid #dee2e6; border-radius: 8px; }
            QCalendarWidget QWidget#qt_calendar_navigationbar { background-color: #ffffff; padding: 8px; border-bottom: 1px solid #dee2e6; }
            QCalendarWidget QToolButton { color: #212529; font-weight: bold; background-color: transparent; padding: 6px; border-radius: 4px; }
            QCalendarWidget QToolButton:hover { background-color: #f1f3f5; }
            QCalendarWidget QMenu { background-color: white; border: 1px solid #dee2e6; }
            QCalendarWidget QSpinBox { background-color: white; border: 1px solid #ced4da; border-radius: 4px; }
            QCalendarWidget QAbstractItemView:enabled { color: #212529; background-color: white; selection-background-color: #0d6efd; selection-color: white; outline: none; }
            QCalendarWidget QAbstractItemView:disabled { color: #adb5bd; }
        """)
        cal.clicked.connect(lambda date: self.on_date_selected(date, dialog))
        layout.addWidget(cal)
        pos = self.mapToGlobal(self.rect().bottomLeft())
        dialog.move(pos.x(), pos.y() + 4)
        dialog.exec()

    def on_date_selected(self, date, dialog):
        self.current_date = date
        self.update_text()
        dialog.accept()

class CheckableComboBox(QComboBox):
    def __init__(self):
        super().__init__()
        self.setModel(QStandardItemModel(self))
        self.setPlaceholderText("Select Networks...")

    def get_checked_items(self):
        checked_items = []
        for i in range(self.count()):
            item = self.model().item(i)
            if item.checkState() == Qt.CheckState.Checked:
                checked_items.append(item.text())
        return checked_items

class BosyCatApp(QMainWindow):
    def __init__(self):
        super().__init__()
        self.setWindowTitle("BosyCat")
        self.resize(900, 650)
        self.setWindowIcon(QIcon("/opt/BosyCat/cat-silhouette-01.svg"))
        self.init_ui()
        self.load_networks()
        self.load_data()

        self.timer = QTimer(self)
        self.timer.timeout.connect(self.load_data)
        self.timer.start(1000)

    def init_ui(self):
        central_widget = QWidget()
        self.setCentralWidget(central_widget)
        main_layout = QVBoxLayout(central_widget)
        main_layout.setSpacing(20)

        filter_layout = QHBoxLayout()
        self.network_combo = CheckableComboBox()
        self.network_combo.setMinimumWidth(220)
        self.date_from = DatePickerButton(QDate.currentDate())
        self.date_to = DatePickerButton(QDate.currentDate())

        filter_layout.addWidget(QLabel("Networks:"))
        filter_layout.addWidget(self.network_combo)
        filter_layout.addSpacing(15)
        filter_layout.addWidget(QLabel("From:"))
        filter_layout.addWidget(self.date_from)
        filter_layout.addSpacing(15)
        filter_layout.addWidget(QLabel("To:"))
        filter_layout.addWidget(self.date_to)
        filter_layout.addStretch()

        summary_layout = QHBoxLayout()
        
        self.lbl_total_all = QLabel("Total: 0 B")
        self.lbl_total_down = QLabel("Download: 0 B")
        self.lbl_total_up = QLabel("Upload: 0 B")
        
        font = QFont("Segoe UI", 13, QFont.Weight.Bold)
        self.lbl_total_all.setFont(font)
        self.lbl_total_down.setFont(font)
        self.lbl_total_up.setFont(font)
        
        self.lbl_total_all.setStyleSheet("color: #1976d2;") 
        self.lbl_total_down.setStyleSheet("color: #d81b60;")
        self.lbl_total_up.setStyleSheet("color: #f57c00;")
        
        summary_layout.addWidget(self.lbl_total_all)
        summary_layout.addSpacing(30)
        summary_layout.addWidget(self.lbl_total_down)
        summary_layout.addSpacing(30)
        summary_layout.addWidget(self.lbl_total_up)
        summary_layout.addStretch()

        self.table = QTableWidget()
        self.table.setColumnCount(4)
        self.table.setHorizontalHeaderLabels(["Application", "Total", "Download", "Upload"])
        self.table.horizontalHeader().setSectionResizeMode(0, QHeaderView.ResizeMode.Stretch)
        self.table.setEditTriggers(QTableWidget.EditTrigger.NoEditTriggers)
        self.table.setSelectionBehavior(QTableWidget.SelectionBehavior.SelectRows)
        self.table.setShowGrid(False)
        self.table.setAlternatingRowColors(True)
        self.table.verticalHeader().setDefaultSectionSize(40)
        self.table.verticalHeader().setVisible(False)
        self.table.setIconSize(QSize(24, 24))

        main_layout.addLayout(filter_layout)
        main_layout.addLayout(summary_layout)
        main_layout.addWidget(self.table)

        self.setStyleSheet("""
            QMainWindow { background-color: #f4f6f9; font-family: 'Segoe UI', Arial, sans-serif; }
            QLabel { font-weight: bold; color: #495057; }
            #datePicker, CheckableComboBox { background-color: white; border: 1px solid #ced4da; border-radius: 6px; padding: 6px 12px; font-size: 13px; color: #212529; text-align: left; }
            #datePicker:hover, CheckableComboBox:focus { border: 1px solid #0d6efd; }
            QTableWidget { background-color: white; border: 1px solid #dee2e6; border-radius: 8px; alternate-background-color: #f8f9fa; selection-background-color: #e9ecef; selection-color: #000; }
            QHeaderView::section { background-color: #ffffff; color: #6c757d; font-weight: bold; border: none; border-bottom: 2px solid #dee2e6; padding: 10px; }
        """)

    def get_db_connection(self):
        return sqlite3.connect(DB_PATH)

    def load_networks(self):
        try:
            with self.get_db_connection() as conn:
                cursor = conn.cursor()
                cursor.execute("SELECT DISTINCT ssid FROM network_metrics")
                db_ssids = [row[0] for row in cursor.fetchall()]

                existing_ssids = []
                for i in range(self.network_combo.count()):
                    existing_ssids.append(self.network_combo.model().item(i).text())

                for ssid in db_ssids:
                    if ssid not in existing_ssids:
                        item = QStandardItem(ssid)
                        item.setFlags(Qt.ItemFlag.ItemIsUserCheckable | Qt.ItemFlag.ItemIsEnabled)
                        item.setCheckState(Qt.CheckState.Checked)
                        self.network_combo.model().appendRow(item)
        except Exception as e:
            pass

    def load_data(self):
        self.load_networks()
        selected_ssids = self.network_combo.get_checked_items()
        if not selected_ssids:
            self.table.setRowCount(0)
            self.lbl_total_all.setText("Total: 0 B")
            self.lbl_total_down.setText("Download: 0 B")
            self.lbl_total_up.setText("Upload: 0 B")
            return

        d_from = self.date_from.date().toString("yyyy-MM-dd")
        d_to = self.date_to.date().toString("yyyy-MM-dd")

        try:
            with self.get_db_connection() as conn:
                cursor = conn.cursor()
                placeholders = ",".join("?" * len(selected_ssids))
                
                query = f"""
                    SELECT app_name,
                           MAX(icon_name) as icon_name,
                           SUM(rx_bytes) as total_rx,
                           SUM(tx_bytes) as total_tx
                    FROM network_metrics
                    WHERE ssid IN ({placeholders})
                      AND date_bucket >= ?
                      AND date_bucket <= ?
                    GROUP BY app_name
                    ORDER BY (total_rx + total_tx) DESC
                """
                params = selected_ssids + [d_from, d_to]
                cursor.execute(query, params)
                rows = cursor.fetchall()

                selected_row = self.table.currentRow()
                self.table.setRowCount(len(rows))
                
                total_down_bytes = 0
                total_up_bytes = 0

                for row_idx, row_data in enumerate(rows):
                    app_name, icon_name, rx_bytes, tx_bytes = row_data
                    total_bytes = rx_bytes + tx_bytes
                    total_down_bytes += rx_bytes
                    total_up_bytes += tx_bytes

                    item_app = QTableWidgetItem(f" {app_name}")
                    
                    if icon_name:
                        if icon_name.startswith("/"):
                            if os.path.exists(icon_name):
                                icon = QIcon(icon_name)
                            elif os.path.exists(icon_name + ".png"):
                                icon = QIcon(icon_name + ".png")
                            elif os.path.exists(icon_name + ".svg"):
                                icon = QIcon(icon_name + ".svg")
                            else:
                                icon = QIcon.fromTheme("application-x-executable")
                        else:
                            icon = QIcon.fromTheme(icon_name)
                            if icon.isNull():
                                icon = QIcon.fromTheme("application-x-executable")
                        item_app.setIcon(icon)
                    else:
                        item_app.setIcon(QIcon.fromTheme("application-x-executable"))
                        
                    self.table.setItem(row_idx, 0, item_app)
                    
                    item_total = QTableWidgetItem(format_size(total_bytes))
                    item_total.setTextAlignment(Qt.AlignmentFlag.AlignCenter)
                    self.table.setItem(row_idx, 1, item_total)
                    
                    item_rx = QTableWidgetItem(format_size(rx_bytes))
                    item_rx.setTextAlignment(Qt.AlignmentFlag.AlignCenter)
                    item_rx.setForeground(QBrush(QColor("#d81b60")))
                    self.table.setItem(row_idx, 2, item_rx)
                    
                    item_tx = QTableWidgetItem(format_size(tx_bytes))
                    item_tx.setTextAlignment(Qt.AlignmentFlag.AlignCenter)
                    item_tx.setForeground(QBrush(QColor("#f57c00")))
                    self.table.setItem(row_idx, 3, item_tx)

                total_all_bytes = total_down_bytes + total_up_bytes
                self.lbl_total_all.setText(f"Total: {format_size(total_all_bytes)}")
                self.lbl_total_down.setText(f"Download: {format_size(total_down_bytes)}")
                self.lbl_total_up.setText(f"Upload: {format_size(total_up_bytes)}")

                if selected_row >= 0 and selected_row < self.table.rowCount():
                    self.table.selectRow(selected_row)

        except Exception as e:
            pass

if __name__ == "__main__":
    app = QApplication(sys.argv)
    
    # الحل الهندسي الرسمي: إضافة مسارات البحث الاحتياطية
    fallback_paths = QIcon.fallbackSearchPaths()
    if "/usr/share/pixmaps" not in fallback_paths:
        fallback_paths.append("/usr/share/pixmaps")
    QIcon.setFallbackSearchPaths(fallback_paths)

    window = BosyCatApp()
    window.show()
    sys.exit(app.exec())