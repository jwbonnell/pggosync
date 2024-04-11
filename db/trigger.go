package db

type Trigger struct {
	TgName       string `db:"tgname"`
	TgIsInternal string `db:"tgisinternal"`
	TgEnabled    bool   `db:"tgenabled"`
	TgConstraint bool   `db:"tgconstraint"`
}
