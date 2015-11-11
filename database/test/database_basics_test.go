package db_test

import (
	db "github.com/fiatjaf/summadb/database"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Basics", func() {
	Context("save operations", func() {
		It("should save a tree", func() {
			Expect(db.SaveTreeAt("/fruits/banana", map[string]interface{}{
				"colour":   "yellow",
				"hardness": "low",
				"_val":     "a fruit.",
			})).To(Succeed())
			Expect(db.GetValueAt("/fruits/banana")).To(BeEquivalentTo(`"a fruit."`))
			Expect(db.GetValueAt("/fruits/banana/colour")).To(BeEquivalentTo(`"yellow"`))
			Expect(db.GetValueAt("/fruits/banana/hardness")).To(BeEquivalentTo(`"low"`))
			_, err := db.GetValueAt("/fruits")
			Expect(err).To(HaveOccurred())
			Expect(db.GetTreeAt("/fruits")).To(Equal(map[string]interface{}{
				"banana": map[string]interface{}{
					"colour":   value("yellow"),
					"hardness": value("low"),
					"_val":     "a fruit.",
				},
			}))
		})

		It("should modify a subvalue of the tree", func() {
			Expect(db.SaveValueAt("/fruits/banana/colour", []byte(`"black-and-yellow"`))).To(Succeed())
			Expect(db.GetValueAt("/fruits/banana/colour")).To(BeEquivalentTo(`"black-and-yellow"`))
			Expect(db.GetValueAt("/fruits/banana/hardness")).To(BeEquivalentTo(`"low"`))
		})

		It("should add a value deeply nested in a tree that doesn't exists", func() {
			Expect(db.SaveValueAt("/fruits/mellon/season", []byte(`"spring"`))).To(Succeed())
			Expect(db.GetValueAt("/fruits/mellon/season")).To(BeEquivalentTo(`"spring"`))
			Expect(db.GetTreeAt("/fruits")).To(Equal(map[string]interface{}{
				"banana": map[string]interface{}{
					"colour":   value("black-and-yellow"),
					"hardness": value("low"),
					"_val":     "a fruit.",
				},
				"mellon": map[string]interface{}{
					"season": value("spring"),
				},
			}))
		})

		It("should add a tree deeply nested like the previous", func() {
			Expect(db.SaveTreeAt("/fruits/orange", map[string]interface{}{
				"colour":   "orange",
				"hardness": "medium",
				"_val":     "name == colour",
			})).To(Succeed())
			Expect(db.GetValueAt("/fruits/orange/colour")).To(BeEquivalentTo(`"orange"`))
			Expect(db.GetValueAt("/fruits/orange")).To(BeEquivalentTo(`"name == colour"`))
			Expect(db.GetTreeAt("/fruits/orange")).To(Equal(map[string]interface{}{
				"_val":     "name == colour",
				"colour":   value("orange"),
				"hardness": value("medium"),
			}))
		})
	})

	Context("delete operations", func() {
		It("should delete a key", func() {
			Expect(db.DeleteAt("/fruits/banana/colour")).To(Succeed())
			Expect(db.GetValueAt("/fruits/orange/colour")).To(BeEquivalentTo(`"orange"`))
			_, err := db.GetValueAt("/fruits/banana/colour")
			Expect(db.GetValueAt("/fruits/banana/colour/_deleted")).To(BeEquivalentTo(""))
			Expect(err).To(HaveOccurred())
		})

		It("should delete a value when setting it to null with a tree", func() {
			Expect(db.SaveTreeAt("/fruits/mellon", map[string]interface{}{
				"colour": "orange",
				"season": nil,
			})).To(Succeed())
			Expect(db.GetValueAt("/fruits/mellon/colour")).To(BeEquivalentTo(`"orange"`))
			_, err := db.GetValueAt("/fruits/mellon/season")
			Expect(err).To(HaveOccurred())

			Expect(db.SaveTreeAt("/fruits", map[string]interface{}{
				"mellon": nil,
			})).To(Succeed())
			_, err = db.GetValueAt("/fruits/mellon/colour")
			Expect(err).To(HaveOccurred())
			_, err = db.GetValueAt("/fruits/mellon")
			Expect(err).To(HaveOccurred())
		})

		It("should delete a tree", func() {
			Expect(db.DeleteAt("/fruits/banana")).To(Succeed())
			Expect(db.GetValueAt("/fruits/orange/colour")).To(BeEquivalentTo(`"orange"`))
			_, err := db.GetValueAt("/fruits/banana/hardness")
			Expect(err).To(HaveOccurred())

			Expect(db.DeleteAt("/fruits")).To(Succeed())
			_, err = db.GetValueAt("/fruits")
			Expect(err).To(HaveOccurred())
			Expect(db.GetValueAt("/fruits/orange/_deleted")).To(BeEquivalentTo(""))
			_, err = db.GetValueAt("/fruits/orange/colour")
			Expect(err).To(HaveOccurred())
			_, err = db.GetValueAt("/fruits/banana/hardness")
			Expect(err).To(HaveOccurred())
		})
	})
})