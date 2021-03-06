package datahub_test

import (
	"fmt"
	"testing"

	"git.kanosolution.net/kano/dbflex"
	"git.kanosolution.net/kano/dbflex/orm"
	"github.com/ariefdarmawan/datahub"

	_ "github.com/ariefdarmawan/flexpg"
	"github.com/eaciit/toolkit"
	cv "github.com/smartystreets/goconvey/convey"
)

var (
	connTxt = "postgres://localhost/testdb?sslmode=disable&binary_parameters=yes"
)

func getConn() (dbflex.IConnection, error) {
	conn, err := dbflex.NewConnectionFromURI(connTxt, nil)
	if err != nil {
		return nil, err
	}

	if err = conn.Connect(); err != nil {
		return nil, err
	}

	conn.SetKeyNameTag("key")
	conn.SetFieldNameTag("json")
	return conn, nil
}

func init() {
	c, e := getConn()
	if e != nil {
		panic(e.Error())
	}
	d := NewDummy(1)
	c.EnsureTable(d.TableName(), []string{"_id"}, d)
}

func TestHubNoPool(t *testing.T) {
	cv.Convey("prepare connection", t, func() {
		conn1, err := getConn()
		cv.So(err, cv.ShouldBeNil)
		defer conn1.Close()

		conn1.Execute(dbflex.From(NewDummy(1).TableName()).Delete(), nil)
		cv.Convey("prepare hub and generate data", func() {
			hub := datahub.NewHub(getConn, false, 0)
			defer hub.Close()

			var err error
			i := 0
			for {
				i++
				if i > 50 {
					break
				}

				d := NewDummy(i)
				err = hub.Insert(d)
				if err != nil {
					break
				}
			}

			cv.So(err, cv.ShouldBeNil)
			cursor := conn1.Cursor(dbflex.From(NewDummy(90).TableName()).Select(), nil)
			cv.So(cursor.Error(), cv.ShouldBeNil)
			defer cursor.Close()
			var res1, res2 []*Dummy
			err = cursor.Fetchs(&res1, 0).Close()
			cv.So(err, cv.ShouldBeNil)
			cv.So(len(res1), cv.ShouldEqual, 50)

			cv.Convey("gets and filter", func() {
				err = hub.Gets(NewDummy(1),
					dbflex.NewQueryParam().SetWhere(dbflex.And(dbflex.Gte("ref1", 10), dbflex.Lte("ref1", 15))),
					&res2)
				cv.So(err, cv.ShouldBeNil)
				cv.So(len(res2), cv.ShouldEqual, 6)

				cv.Convey("update", func() {
					for _, r := range res2 {
						if r.Ref1 > 12 && r.Ref1 <= 14 {
							r.Ref2 = 100
						}
						err = hub.Update(r)
						if err != nil {
							break
						}
					}

					cv.So(err, cv.ShouldBeNil)
					hub.Gets(NewDummy(1), dbflex.NewQueryParam().SetWhere(dbflex.Eq("ref2", 100)), &res2)
					cv.So(len(res2), cv.ShouldEqual, 2)

					cv.Convey("save", func() {
						//-- update ref2 to 200 for 13 and 14
						for _, r := range res2 {
							r.Ref2 = 200
							hub.Save(r)
						}

						//-- insert ref2 200 for 100 - 105
						for i := 100; i <= 105; i++ {
							r := NewDummy(i)
							r.Ref2 = 200
							err = hub.Save(r)
							if err != nil {
								break
							}
						}

						cv.So(err, cv.ShouldBeNil)
						hub.Gets(NewDummy(1), dbflex.NewQueryParam().SetWhere(dbflex.Eq("ref2", 200)), &res2)
						cv.So(len(res2), cv.ShouldEqual, 8)

						cv.Convey("delete", func() {
							//-- delete 30 - 39
							for i := 30; i <= 39; i++ {
								d := NewDummy(i)
								if err = hub.Delete(d); err != nil {
									break
								}
							}

							hub.Gets(NewDummy(1), nil, &res2)
							cv.So(len(res2), cv.ShouldEqual, 46)

							cv.Convey("get", func() {
								d1 := NewDummy(20)
								d2 := NewDummy(20)

								d1.Ref2 = 85
								hub.Save(d1)

								err = hub.Get(d2)
								cv.So(err, cv.ShouldBeNil)
								cv.So(d1.Ref2, cv.ShouldEqual, d2.Ref2)

								cv.Convey("aggregate", func() {
									//-- lets update 6 to 10
									var res3 []*Dummy
									hub.Gets(NewDummy(1),
										dbflex.NewQueryParam().SetWhere(dbflex.And(dbflex.Gte("ref1", 6), dbflex.Lte("ref1", 10))),
										&res3)
									for _, d := range res3 {
										d.Ref2 = 150
										hub.Save(d)
									}

									ms := []toolkit.M{}
									err = hub.PopulateByParm(NewDummy(1).TableName(),
										dbflex.NewQueryParam().
											SetWhere(dbflex.And(dbflex.Gte("ref1", 6), dbflex.Lte("ref1", 10))).
											SetAggr(dbflex.NewAggrItem("ref1", dbflex.AggrSum, "ref1"),
												dbflex.NewAggrItem("ref2", dbflex.AggrSum, "ref2")),
										&ms)
									cv.So(err, cv.ShouldBeNil)
									cv.So(ms[0].GetInt("ref1"), cv.ShouldEqual, 6+7+8+9+10)
									cv.So(ms[0].GetInt("ref2"), cv.ShouldEqual, 750)
								})
							})
						})
					})
				})
			})
		})
	})
}

func TestHubWithPool(t *testing.T) {
	cv.Convey("prepare connection", t, func() {
		conn1, err := getConn()
		cv.So(err, cv.ShouldBeNil)
		defer conn1.Close()

		conn1.Execute(dbflex.From(NewDummy(1).TableName()).Delete(), nil)
		cv.Convey("prepare hub and generate data", func() {
			hub := datahub.NewHub(getConn, true, 10)
			defer hub.Close()

			var err error
			i := 0
			for {
				i++
				if i > 50 {
					break
				}

				d := NewDummy(i)
				err = hub.Insert(d)
				if err != nil {
					break
				}
			}

			cv.So(err, cv.ShouldBeNil)
			cursor := conn1.Cursor(dbflex.From(NewDummy(90).TableName()).Select(), nil)
			cv.So(cursor.Error(), cv.ShouldBeNil)
			defer cursor.Close()
			var res1, res2 []*Dummy
			err = cursor.Fetchs(&res1, 0).Close()
			cv.So(err, cv.ShouldBeNil)
			cv.So(len(res1), cv.ShouldEqual, 50)

			cv.Convey("gets and filter", func() {
				err = hub.Gets(NewDummy(1),
					dbflex.NewQueryParam().SetWhere(dbflex.And(dbflex.Gte("ref1", 10), dbflex.Lte("ref1", 15))),
					&res2)
				cv.So(err, cv.ShouldBeNil)
				cv.So(len(res2), cv.ShouldEqual, 6)

				cv.Convey("update", func() {
					for _, r := range res2 {
						if r.Ref1 > 12 && r.Ref1 <= 14 {
							r.Ref2 = 100
						}
						err = hub.Update(r)
						if err != nil {
							break
						}
					}

					cv.So(err, cv.ShouldBeNil)
					hub.Gets(NewDummy(1), dbflex.NewQueryParam().SetWhere(dbflex.Eq("ref2", 100)), &res2)
					cv.So(len(res2), cv.ShouldEqual, 2)

					cv.Convey("save", func() {
						//-- update ref2 to 200 for 13 and 14
						for _, r := range res2 {
							r.Ref2 = 200
							hub.Save(r)
						}

						//-- insert ref2 200 for 100 - 105
						for i := 100; i <= 105; i++ {
							r := NewDummy(i)
							r.Ref2 = 200
							err = hub.Save(r)
							if err != nil {
								break
							}
						}

						cv.So(err, cv.ShouldBeNil)
						hub.Gets(NewDummy(1), dbflex.NewQueryParam().SetWhere(dbflex.Eq("ref2", 200)), &res2)
						cv.So(len(res2), cv.ShouldEqual, 8)

						cv.Convey("delete", func() {
							//-- delete 30 - 39
							for i := 30; i <= 39; i++ {
								d := NewDummy(i)
								if err = hub.Delete(d); err != nil {
									break
								}
							}

							hub.Gets(NewDummy(1), nil, &res2)
							cv.So(len(res2), cv.ShouldEqual, 46)

							cv.Convey("get", func() {
								d1 := NewDummy(20)
								d2 := NewDummy(20)

								d1.Ref2 = 85
								hub.Save(d1)

								err = hub.Get(d2)
								cv.So(err, cv.ShouldBeNil)
								cv.So(d1.Ref2, cv.ShouldEqual, d2.Ref2)
							})
						})
					})
				})
			})
		})
	})
}

func TestHubTrxCommit(t *testing.T) {
	h := datahub.NewHub(getConn, true, 10)
	data := NewDummy(1024)
	h.DeleteQuery(data, nil)

	cv.Convey("start tx", t, func() {
		ht, err := h.BeginTx()
		cv.So(err, cv.ShouldBeNil)

		cv.Convey("insert data", func() {
			err = ht.Save(data)
			cv.So(err, cv.ShouldBeNil)

			cv.Convey("get data within tx", func() {
				dtx := new(Dummy)
				ht.GetByID(dtx, "User-1024")
				cv.So(dtx.Name, cv.ShouldEqual, data.Name)

				cv.Convey("get data outside tx", func() {
					dtx = new(Dummy)
					h.GetByID(dtx, "User-1024")
					cv.So(dtx.Name, cv.ShouldEqual, "")

					cv.Convey("commit", func() {
						e := ht.Commit()
						cv.So(e, cv.ShouldBeNil)

						cv.Convey("validate commit", func() {
							h.GetByID(dtx, "User-1024")
							cv.So(dtx.Name, cv.ShouldEqual, data.Name)
						})
					})
				})
			})
		})
	})
}

func TestHubTrxRollback(t *testing.T) {
	h := datahub.NewHub(getConn, true, 10)
	data := NewDummy(1025)

	cv.Convey("start tx", t, func() {
		ht, err := h.BeginTx()
		cv.So(err, cv.ShouldBeNil)

		cv.Convey("insert data", func() {
			err = ht.Save(data)
			cv.So(err, cv.ShouldBeNil)

			cv.Convey("get data within tx", func() {
				dtx := new(Dummy)
				ht.GetByID(dtx, "User-1025")
				cv.So(dtx.Name, cv.ShouldEqual, data.Name)

				cv.Convey("get data outside tx", func() {
					dtx = new(Dummy)
					h.GetByID(dtx, "User-1025")
					cv.So(dtx.Name, cv.ShouldEqual, "")

					cv.Convey("rollback", func() {
						e := ht.Rollback()
						cv.So(e, cv.ShouldBeNil)

						cv.Convey("validate rollback", func() {
							h.GetByID(dtx, "User-1025")
							cv.So(dtx.Name, cv.ShouldEqual, "")
						})
					})
				})
			})
		})
	})
}

func NewDummy(i int) *Dummy {
	d := new(Dummy)
	d.ID = fmt.Sprintf("User-%d", i)
	d.Name = fmt.Sprintf("Employee %d", i)
	d.Ref1 = i
	d.Ref2 = 0
	d.SetThis(d)
	return d
}

type Dummy struct {
	orm.DataModelBase `bson:"-" json:"-" ecname:"-"`

	ID   string `bson:"_id" json:"_id" sqlname:"_id" key:"1"`
	Name string
	Ref1 int
	Ref2 int
}

func (d *Dummy) TableName() string {
	return "DatahubTestTable"
}

func (d *Dummy) SetID(keys ...interface{}) {
	d.ID = keys[0].(string)
}
