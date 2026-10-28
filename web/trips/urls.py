from django.urls import path

from trips import views

urlpatterns = [
    path("", views.home, name="home"),
    path("api/v1/plan", views.plan_api, name="plan-api"),
    path("health", views.health, name="health"),
    path("health/ready", views.ready, name="ready"),
]
