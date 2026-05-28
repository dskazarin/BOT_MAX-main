package main

import (
    "log"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "patient_cabinet.html")
    })
    log.Println("🚀 Личный кабинет пациента запущен на http://localhost:8082")
    log.Fatal(http.ListenAndServe(":8082", nil))
}
