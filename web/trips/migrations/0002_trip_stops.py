from django.db import migrations, models


class Migration(migrations.Migration):
    dependencies = [
        ("trips", "0001_initial"),
    ]

    operations = [
        migrations.AddField(
            model_name="trip",
            name="stops",
            field=models.JSONField(default=list),
        ),
    ]
