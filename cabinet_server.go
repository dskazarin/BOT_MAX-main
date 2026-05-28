package main

import (
    "fmt"
    "log"
    "net/http"
)

func main() {
    // Обслуживаем статические файлы
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/" {
            http.ServeFile(w, r, "patient_cabinet_full.html")
            return
        }
        http.ServeFile(w, r, r.URL.Path[1:])
    })
    
    fmt.Println("🌐 Сервер запущен на http://localhost:8082")
    fmt.Println("📋 Личный кабинет: http://localhost:8082")
    
    log.Fatal(http.ListenAndServe(":8082", nil))
}
